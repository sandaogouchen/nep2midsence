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
  assertVisible: "assert", assertText: "assert", assertExists: "assert", expect: "assert",
  getText: "extract", getInnerText: "extract", getAttribute: "extract",
  create: "lifecycle", close: "lifecycle",
  login: "composite",
};

const TEST_HOOKS = new Set(["beforeEach", "afterEach", "beforeAll", "afterAll"]);
const TEST_BLOCKS = new Set(["describe", "it", "test"]);

// ── Types ──────────────────────────────────────────────────────────────────────
interface ImportInfo {
  module: string;
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
  params: Array<{ name: string; type: string }>;
  startLine: number;
  endLine: number;
  bodyText: string;
}

interface CallInfo {
  callee: string;
  arguments: string[];
  startLine: number;
  isAwait: boolean;
  isNepAPI: boolean;
  nepAPIType: string;
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
  fileName: string;
  language: "typescript" | "javascript";
  imports: ImportInfo[];
  topLevelVariables: TopLevelVariable[];
  functions: FunctionInfo[];
  allCalls: CallInfo[];
  testStructure: { describes: DescribeBlock[] };
  rawLineCount: number;
  parseErrors: string[];
  warnings: string[];
  extractedAt: string;
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
      calls.push({
        callee,
        arguments: argTexts(target as ts.CallExpression, src),
        startLine: lineOf(src, target.getStart()),
        isAwait: awaited,
        isNepAPI: nep.isNepAPI,
        nepAPIType: nep.nepAPIType,
      });
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

  if (clause) {
    if (clause.name) { isDef = true; specifiers.push(clause.name.text); }
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
  return { module, specifiers, isDefault: isDef, isNamespace: isNs };
}

// ── Params Extractor ───────────────────────────────────────────────────────────
function extractParams(
  params: ts.NodeArray<ts.ParameterDeclaration>, src: ts.SourceFile
): Array<{ name: string; type: string }> {
  return params.map((p) => ({
    name: p.name.getText(src),
    type: p.type ? p.type.getText(src) : "",
  }));
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
        topLevelVariables.push({
          name: decl.name.getText(src),
          kind,
          type: typeText(decl.type, src),
          initializer: decl.initializer ? decl.initializer.getText(src) : "",
          startLine: lineOf(src, stmt.getStart()),
        });
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
      });
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
        });
      }
    }
  }

  return {
    filePath: absolutePath,
    fileName: path.basename(filePath),
    language,
    imports,
    topLevelVariables,
    functions,
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
