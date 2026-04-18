import * as ts from "typescript";
import * as fs from "fs";
import * as path from "path";

// ── NEP API Mapping Table ──────────────────────────────────────────────────────
const NEP_API_MAP: Record<string, string> = {
  navigate: "navigate", goto: "navigate", navigateTo: "navigate",
  click: "click", dblclick: "click", clickLogin: "click", clickCreateCampaign: "click",
  sendKeys: "input", type: "input", fill: "input",
  fillUsername: "input", fillPassword: "input", setBudget: "input", selectObjective: "input",
  findElement: "findElement", querySelector: "findElement",
  findElements: "findElements", querySelectorAll: "findElements",
  waitForElement: "waitFor", waitForSelector: "waitFor",
  waitForNavigation: "waitFor", waitForTimeout: "waitFor",
  assertVisible: "assert", assertText: "assert", assertExists: "assert",
  getText: "extract", getInnerText: "extract", getAttribute: "extract",
  create: "lifecycle", close: "lifecycle",
  login: "composite",
};

const TEST_HOOKS = new Set(["beforeEach", "afterEach", "beforeAll", "afterAll"]);
const TEST_BLOCKS = new Set(["describe", "it", "test"]);
const DEFAULT_INFRA_ROOTS = new Set(["page", "browser", "context", "console", "json", "math", "promise", "process", "window", "document"]);
const BUSINESS_ROOT_PATTERNS = [/(?:Page|Actions|Module|Helper)$/i];
const ELEMENT_LIKE_PROPERTY_PATTERNS = [/(?:Btn|Button|Input|Select|Locator|Element)$/i];
const INFRA_TERMINAL_METHODS = new Set(["click", "dblclick", "hover", "focus", "blur", "check", "uncheck", "fill", "type", "press", "locator", "nth", "first", "last"]);

// ── Types ──────────────────────────────────────────────────────────────────────
interface ImportInfo {
  module: string;
  // For Go-side TSBridge compatibility
  names?: string;
  alias?: string;
  line?: number;
  isNep?: boolean;

  // Extra info (optional, kept for debugging)
  specifiers: string[];
  isDefault: boolean;
  isNamespace: boolean;
}

interface TopLevelVariable {
  name: string;
  kind: "const" | "let" | "var";
  type: string;
  initializer: string;
  startLine: number;
}

interface FunctionInfo {
  name: string;
  isAsync: boolean;
  isExported: boolean;
  isTest: boolean;
  isHelper: boolean;
  testType: string;
  // For Go-side TSBridge compatibility
  params: string[];
  startLine: number;
  endLine: number;
  bodyText: string;
  doc?: string;
  receiver?: string;
}

interface CallInfo {
  callee: string;
  // For Go-side TSBridge compatibility
  args?: string[];

  // Extra info (optional)
  arguments: string[];
  startLine: number;
  isAwait: boolean;
  isNepAPI: boolean;
  nepAPIType: string;

  // For Go-side TSBridge compatibility
  receiver?: string;
  fullReceiver?: string;
  funcName?: string;
  ownerRoot?: string;
  ownerKind?: string;
  ownerSource?: string;
  ownerFile?: string;
  line?: number;
  isNep?: boolean;
  isChained?: boolean;
  inFunc?: string;
  isWrapperCall?: boolean;
}

interface ConstInfo {
  name: string;
  value: string;
  type?: string;
  line: number;
}

interface VarInfo {
  name: string;
  value: string;
  type?: string;
  line: number;
}

interface HookBlock {
  startLine: number;
  endLine: number;
  bodyText: string;
}

interface TestEntry {
  name: string;
  startLine: number;
  endLine: number;
  calls: CallInfo[];
  isAsync: boolean;
}

interface DescribeBlock {
  name: string;
  startLine: number;
  endLine: number;
  beforeEach: HookBlock | null;
  afterEach: HookBlock | null;
  beforeAll: HookBlock | null;
  afterAll: HookBlock | null;
  tests: TestEntry[];
  nestedDescribes: DescribeBlock[];
}

interface FileAnalysis {
  filePath: string;
  // Go-side TSBridge expects these keys:
  imports: ImportInfo[];
  functions: FunctionInfo[];
  calls: CallInfo[];
  constants: ConstInfo[];
  variables: VarInfo[];

  // Extra fields (not required by TSBridge, kept for debugging/future)
  fileName?: string;
  language?: "typescript" | "javascript";
  topLevelVariables?: TopLevelVariable[];
  allCalls?: CallInfo[];
  testStructure?: { describes: DescribeBlock[] };
  rawLineCount?: number;
  parseErrors?: string[];
  warnings?: string[];
  extractedAt?: string;
}

// ── Utility Helpers ────────────────────────────────────────────────────────────
function lineOf(sourceFile: ts.SourceFile, pos: number): number {
  return sourceFile.getLineAndCharacterOfPosition(pos).line + 1;
}

function calleeText(node: ts.CallExpression, src: ts.SourceFile): string {
  const expr = node.expression;
  if (ts.isIdentifier(expr)) return expr.text;
  if (ts.isPropertyAccessExpression(expr)) return expr.getText(src);
  if (ts.isElementAccessExpression(expr)) return expr.getText(src);
  return expr.getText(src);
}

function terminalMethodName(callee: string): string {
  const parts = callee.split(".");
  return parts[parts.length - 1];
}

function extractOwnerRoot(fullReceiver: string): string {
  const parts = fullReceiver.split(".").map((part) => part.trim()).filter(Boolean);
  for (const part of parts) {
    if (part === "this") continue;
    return part;
  }
  return "";
}

function ownerPropertySegments(fullReceiver: string): string[] {
  const parts = fullReceiver.split(".").map((part) => part.trim()).filter((part) => part !== "" && part !== "this");
  return parts.slice(1);
}

function classifyOwner(fullReceiver: string, funcName: string): { ownerRoot: string; ownerKind: string; ownerSource: string } {
  const ownerRoot = extractOwnerRoot(fullReceiver);
  if (!ownerRoot) {
    if (["expect", "assert", "log", "info", "warn", "error", "debug", "stringify", "parse"].includes(funcName)) {
      return { ownerRoot: "", ownerKind: "infrastructure", ownerSource: "safety_no_receiver" };
    }
    return { ownerRoot: "", ownerKind: "unknown", ownerSource: "no_owner_root" };
  }

  if (DEFAULT_INFRA_ROOTS.has(ownerRoot.toLowerCase())) {
    return { ownerRoot, ownerKind: "infrastructure", ownerSource: "known_infra_root" };
  }

  if (INFRA_TERMINAL_METHODS.has(funcName)) {
    for (const seg of ownerPropertySegments(fullReceiver)) {
      if (ELEMENT_LIKE_PROPERTY_PATTERNS.some((pattern) => pattern.test(seg))) {
        return { ownerRoot, ownerKind: "infrastructure", ownerSource: "element_like_property" };
      }
    }
  }

  if (BUSINESS_ROOT_PATTERNS.some((pattern) => pattern.test(ownerRoot))) {
    return { ownerRoot, ownerKind: "business", ownerSource: "business_name_pattern" };
  }

  return { ownerRoot, ownerKind: "unknown", ownerSource: "fallback_unknown" };
}

function resolveNep(callee: string): { isNepAPI: boolean; nepAPIType: string } {
  const method = terminalMethodName(callee);
  const cat = NEP_API_MAP[method];
  return cat ? { isNepAPI: true, nepAPIType: cat } : { isNepAPI: false, nepAPIType: "" };
}

function argTexts(node: ts.CallExpression, src: ts.SourceFile): string[] {
  return node.arguments.map((a) => a.getText(src));
}

function bodyTextOf(node: ts.Node, sourceText: string): string {
  return sourceText.substring(node.getStart(), node.getEnd());
}

function varKind(flags: ts.NodeFlags): "const" | "let" | "var" {
  if (flags & ts.NodeFlags.Const) return "const";
  if (flags & ts.NodeFlags.Let) return "let";
  return "var";
}

function typeText(typeNode: ts.TypeNode | undefined, src: ts.SourceFile): string {
  return typeNode ? typeNode.getText(src) : "";
}

function isExported(node: ts.Node): boolean {
  const mods = ts.canHaveModifiers(node) ? ts.getModifiers(node) : undefined;
  return mods?.some((m) => m.kind === ts.SyntaxKind.ExportKeyword) ?? false;
}

function isAsync(node: ts.Node): boolean {
  const mods = ts.canHaveModifiers(node) ? ts.getModifiers(node) : undefined;
  return mods?.some((m) => m.kind === ts.SyntaxKind.AsyncKeyword) ?? false;
}

function firstStringArg(node: ts.CallExpression): string {
  const a = node.arguments[0];
  if (a && (ts.isStringLiteral(a) || ts.isNoSubstitutionTemplateLiteral(a))) return a.text;
  return "<dynamic>";
}

function callbackBody(node: ts.CallExpression): ts.Node | undefined {
  for (const arg of node.arguments) {
    if (ts.isFunctionExpression(arg) || ts.isArrowFunction(arg)) return arg.body;
  }
  return undefined;
}

function callbackIsAsync(node: ts.CallExpression): boolean {
  for (const arg of node.arguments) {
    if (ts.isFunctionExpression(arg) || ts.isArrowFunction(arg)) return isAsync(arg);
  }
  return false;
}

// ── Call Collector ──────────────────────────────────────────────────────────────
function collectCalls(node: ts.Node, src: ts.SourceFile, sourceText: string): CallInfo[] {
  const calls: CallInfo[] = [];
  const seenBySpan = new Map<string, number>();
  function walk(n: ts.Node) {
    let target = n;
    let awaited = false;
    if (ts.isAwaitExpression(n) && ts.isCallExpression(n.expression)) {
      target = n.expression;
      awaited = true;
    }
    if (ts.isCallExpression(target)) {
      const callee = calleeText(target as ts.CallExpression, src);
      const nep = resolveNep(callee);

      const parts = callee.split(".");
      const funcName = terminalMethodName(callee);
      const fullReceiver = parts.length > 1 ? parts.slice(0, -1).join(".") : "";
      const receiver = parts.length > 1 ? parts[parts.length - 2] : "";
      const isChained = parts.length > 1;
      const isWrapperCall = parts.length > 2;
      const owner = classifyOwner(fullReceiver, funcName);

      const startLine = lineOf(src, target.getStart());
      const key = `${target.getStart()}-${target.getEnd()}-${callee}`;
      if (seenBySpan.has(key)) {
        const existing = calls[seenBySpan.get(key)!];
        existing.isAwait = existing.isAwait || awaited;
        return;
      }

      calls.push({
        callee,
        args: argTexts(target as ts.CallExpression, src),
        arguments: argTexts(target as ts.CallExpression, src),
        startLine,
        isAwait: awaited,
        isNepAPI: nep.isNepAPI,
        nepAPIType: nep.nepAPIType,

        // TSBridge-compatible fields
        receiver,
        fullReceiver,
        funcName,
        ownerRoot: owner.ownerRoot,
        ownerKind: owner.ownerKind,
        ownerSource: owner.ownerSource,
        ownerFile: "",
        line: startLine,
        isNep: nep.isNepAPI,
        isChained,
        inFunc: "",
        isWrapperCall,
      });
      seenBySpan.set(key, calls.length - 1);
    }
    ts.forEachChild(n, walk);
  }
  ts.forEachChild(node, walk);
  return calls;
}

// ── Test Structure Builder ─────────────────────────────────────────────────────
function buildDescribe(
  node: ts.CallExpression, src: ts.SourceFile, sourceText: string
): DescribeBlock {
  const block: DescribeBlock = {
    name: firstStringArg(node),
    startLine: lineOf(src, node.getStart()),
    endLine: lineOf(src, node.getEnd()),
    beforeEach: null, afterEach: null, beforeAll: null, afterAll: null,
    tests: [],
    nestedDescribes: [],
  };

  const body = callbackBody(node);
  if (!body) return block;

  const stmts = ts.isBlock(body) ? body.statements : [];
  for (const stmt of stmts) {
    if (!ts.isExpressionStatement(stmt)) continue;
    const expr = stmt.expression;
    if (!ts.isCallExpression(expr)) continue;
    const callee = calleeText(expr, src);
    const method = terminalMethodName(callee);

    if (method === "describe") {
      block.nestedDescribes.push(buildDescribe(expr, src, sourceText));
    } else if (method === "it" || method === "test") {
      const cbBody = callbackBody(expr);
      block.tests.push({
        name: firstStringArg(expr),
        startLine: lineOf(src, expr.getStart()),
        endLine: lineOf(src, expr.getEnd()),
        calls: cbBody ? collectCalls(cbBody, src, sourceText) : [],
        isAsync: callbackIsAsync(expr),
      });
    } else if (TEST_HOOKS.has(method)) {
      const hookBody = callbackBody(expr);
      const hookBlock: HookBlock = {
        startLine: lineOf(src, expr.getStart()),
        endLine: lineOf(src, expr.getEnd()),
        bodyText: hookBody ? bodyTextOf(hookBody, sourceText) : "",
      };
      if (method === "beforeEach") block.beforeEach = hookBlock;
      else if (method === "afterEach") block.afterEach = hookBlock;
      else if (method === "beforeAll") block.beforeAll = hookBlock;
      else if (method === "afterAll") block.afterAll = hookBlock;
    }
  }
  return block;
}

// ── Imports Extractor ──────────────────────────────────────────────────────────
function extractImport(node: ts.ImportDeclaration, src: ts.SourceFile): ImportInfo {
  const module = (node.moduleSpecifier as ts.StringLiteral).text;
  const clause = node.importClause;
  const specifiers: string[] = [];
  let isDef = false;
  let isNs = false;
  let alias = "";

  if (clause) {
    if (clause.name) { isDef = true; specifiers.push(clause.name.text); alias = clause.name.text; }
    const bindings = clause.namedBindings;
    if (bindings) {
      if (ts.isNamespaceImport(bindings)) {
        isNs = true;
        specifiers.push(bindings.name.text);
      } else if (ts.isNamedImports(bindings)) {
        for (const el of bindings.elements) specifiers.push(el.name.text);
      }
    }
  }

  const line = lineOf(src, node.getStart());
  const names = specifiers.join(",");
  const isNep = module.toLowerCase().includes("nep");
  return { module, names, alias, line, isNep, specifiers, isDefault: isDef, isNamespace: isNs };
}

// ── Params Extractor ───────────────────────────────────────────────────────────
function extractParams(
  params: ts.NodeArray<ts.ParameterDeclaration>, src: ts.SourceFile
): string[] {
  return params.map((p) => p.name.getText(src));
}

function flattenDescribeTests(describes: DescribeBlock[], out: FunctionInfo[]) {
  for (const d of describes) {
    for (const t of d.tests) {
      out.push({
        name: t.name,
        isAsync: t.isAsync,
        isExported: false,
        isTest: true,
        isHelper: false,
        testType: "it",
        params: [],
        startLine: t.startLine,
        endLine: t.endLine,
        bodyText: "",
        doc: "",
        receiver: "",
      });
    }
    flattenDescribeTests(d.nestedDescribes, out);
  }
}

// ── Main File Analyser ─────────────────────────────────────────────────────────
function analyseFile(filePath: string): FileAnalysis {
  const absolutePath = path.resolve(filePath);
  const sourceText = fs.readFileSync(absolutePath, "utf-8");
  const ext = path.extname(filePath).toLowerCase();
  const scriptKind =
    ext === ".tsx" ? ts.ScriptKind.TSX :
    ext === ".jsx" ? ts.ScriptKind.JSX :
    ext === ".js"  ? ts.ScriptKind.JS  : ts.ScriptKind.TS;

  const src = ts.createSourceFile(
    path.basename(filePath), sourceText, ts.ScriptTarget.Latest, true, scriptKind
  );

  const language: "typescript" | "javascript" = ext === ".js" || ext === ".jsx" ? "javascript" : "typescript";
  const rawLineCount = sourceText.split("\n").length;
  const warnings: string[] = [];
  const parseErrors: string[] = [];

  if (rawLineCount > 10000) warnings.push("large_file");

  // Collect parse diagnostics (ts.createSourceFile stores them internally).
  const diags = (src as any).parseDiagnostics as ts.DiagnosticWithLocation[] | undefined;
  if (diags) {
    for (const d of diags) {
      const msg = ts.flattenDiagnosticMessageText(d.messageText, " ");
      const line = lineOf(src, d.start ?? 0);
      parseErrors.push(`Line ${line}: ${msg}`);
    }
  }

  const imports: ImportInfo[] = [];
  const topLevelVariables: TopLevelVariable[] = [];
  const functions: FunctionInfo[] = [];
  const allCalls: CallInfo[] = [];
  const describes: DescribeBlock[] = [];
  const constants: ConstInfo[] = [];
  const variables: VarInfo[] = [];

  // Global call collection
  allCalls.push(...collectCalls(src, src, sourceText));

  // Top-level walk
  for (const stmt of src.statements) {
    // Imports
    if (ts.isImportDeclaration(stmt)) {
      imports.push(extractImport(stmt, src));
      continue;
    }

    // Variable statements
    if (ts.isVariableStatement(stmt)) {
      const kind = varKind(stmt.declarationList.flags);
      for (const decl of stmt.declarationList.declarations) {
        const initializer = decl.initializer ? decl.initializer.getText(src) : "";
        const startLine = lineOf(src, stmt.getStart());
        topLevelVariables.push({
          name: decl.name.getText(src),
          kind,
          type: typeText(decl.type, src),
          initializer,
          startLine,
        });

        // TSBridge-compatible constants/variables
        if (kind === "const") {
          constants.push({
            name: decl.name.getText(src),
            value: initializer,
            type: typeText(decl.type, src),
            line: startLine,
          });
        } else {
          variables.push({
            name: decl.name.getText(src),
            value: initializer,
            type: typeText(decl.type, src),
            line: startLine,
          });
        }
      }
    }

    // Function declarations
    if (ts.isFunctionDeclaration(stmt) && stmt.name) {
      functions.push({
        name: stmt.name.text,
        isAsync: isAsync(stmt),
        isExported: isExported(stmt),
        isTest: false,
        isHelper: true,
        testType: "",
        params: extractParams(stmt.parameters, src),
        startLine: lineOf(src, stmt.getStart()),
        endLine: lineOf(src, stmt.getEnd()),
        bodyText: stmt.body ? bodyTextOf(stmt.body, sourceText) : "",
        doc: "",
        receiver: "",
      });
    }

    if (ts.isClassDeclaration(stmt) && stmt.name) {
      const receiver = stmt.name.text;
      for (const member of stmt.members) {
        if (ts.isMethodDeclaration(member) && member.name) {
          functions.push({
            name: member.name.getText(src),
            isAsync: isAsync(member),
            isExported: isExported(stmt),
            isTest: false,
            isHelper: true,
            testType: "",
            params: extractParams(member.parameters, src),
            startLine: lineOf(src, member.getStart()),
            endLine: lineOf(src, member.getEnd()),
            bodyText: member.body ? bodyTextOf(member.body, sourceText) : "",
            doc: "",
            receiver,
          });
          continue;
        }
        if (!ts.isPropertyDeclaration(member) || !member.name || !member.initializer) {
          continue;
        }
        if (!ts.isArrowFunction(member.initializer) && !ts.isFunctionExpression(member.initializer)) {
          continue;
        }
        const fn = member.initializer;
        functions.push({
          name: member.name.getText(src),
          isAsync: isAsync(fn),
          isExported: isExported(stmt),
          isTest: false,
          isHelper: true,
          testType: "",
          params: extractParams(fn.parameters, src),
          startLine: lineOf(src, member.getStart()),
          endLine: lineOf(src, member.getEnd()),
          bodyText: fn.body ? bodyTextOf(fn.body, sourceText) : "",
          doc: "",
          receiver,
        });
      }
    }

    // Exported / top-level const arrow functions
    if (ts.isVariableStatement(stmt)) {
      for (const decl of stmt.declarationList.declarations) {
        if (
          decl.initializer &&
          (ts.isArrowFunction(decl.initializer) || ts.isFunctionExpression(decl.initializer))
        ) {
          const fn = decl.initializer;
          functions.push({
            name: decl.name.getText(src),
            isAsync: isAsync(fn),
            isExported: isExported(stmt),
            isTest: false,
            isHelper: true,
            testType: "",
            params: extractParams(fn.parameters, src),
            startLine: lineOf(src, stmt.getStart()),
            endLine: lineOf(src, stmt.getEnd()),
            bodyText: fn.body ? bodyTextOf(fn.body, sourceText) : "",
            doc: "",
            receiver: "",
          });
        }
      }
    }

    // Test structure: top-level describe / it / test
    if (ts.isExpressionStatement(stmt) && ts.isCallExpression(stmt.expression)) {
      const callee = calleeText(stmt.expression, src);
      const method = terminalMethodName(callee);

      if (method === "describe") {
        describes.push(buildDescribe(stmt.expression, src, sourceText));
      } else if (method === "it" || method === "test") {
        // Standalone test outside a describe
        const cbBody = callbackBody(stmt.expression);
        functions.push({
          name: firstStringArg(stmt.expression),
          isAsync: callbackIsAsync(stmt.expression),
          isExported: false,
          isTest: true,
          isHelper: false,
          testType: method,
          params: [],
          startLine: lineOf(src, stmt.getStart()),
          endLine: lineOf(src, stmt.getEnd()),
          bodyText: cbBody ? bodyTextOf(cbBody, sourceText) : "",
          doc: "",
          receiver: "",
        });
      }
    }
  }

  // Add tests nested inside describe blocks into the functions list so Go-side
  // call-chain grouping can work.
  flattenDescribeTests(describes, functions);

  // TSBridge-compatible call list
  const calls = allCalls;

  return {
    filePath: absolutePath,
    imports,
    functions,
    calls,
    constants,
    variables,

    // extra/debug
    fileName: path.basename(filePath),
    language,
    topLevelVariables,
    allCalls,
    testStructure: { describes },
    rawLineCount,
    parseErrors,
    warnings,
    extractedAt: new Date().toISOString(),
  };
}

// ── CLI Entry Point ────────────────────────────────────────────────────────────
function main(): void {
  const args = process.argv.slice(2);
  if (args.length === 0) {
    process.stderr.write("Usage: ts-ast-extractor <file1.ts> [file2.ts ...]\n");
    process.exit(1);
  }

  // Validate all files exist before processing
  for (const filePath of args) {
    if (!fs.existsSync(filePath)) {
      process.stderr.write(`Error: file not found: ${filePath}\n`);
      process.exit(1);
    }
  }

  const results: FileAnalysis[] = args.map((f) => analyseFile(f));
  const output = args.length === 1 ? results[0] : results;
  process.stdout.write(JSON.stringify(output, null, 2) + "\n");
}

main();
