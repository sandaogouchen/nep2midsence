"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
const ts = __importStar(require("typescript"));
const fs = __importStar(require("fs"));
const path = __importStar(require("path"));
// ── NEP API Mapping Table ──────────────────────────────────────────────────────
const NEP_API_MAP = {
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
// ── Utility Helpers ────────────────────────────────────────────────────────────
function lineOf(sourceFile, pos) {
    return sourceFile.getLineAndCharacterOfPosition(pos).line + 1;
}
function calleeText(node, src) {
    const expr = node.expression;
    if (ts.isIdentifier(expr))
        return expr.text;
    if (ts.isPropertyAccessExpression(expr))
        return expr.getText(src);
    if (ts.isElementAccessExpression(expr))
        return expr.getText(src);
    return expr.getText(src);
}
function terminalMethodName(callee) {
    const parts = callee.split(".");
    return parts[parts.length - 1];
}
function extractOwnerRoot(fullReceiver) {
    const parts = fullReceiver.split(".").map((part) => part.trim()).filter(Boolean);
    for (const part of parts) {
        if (part === "this")
            continue;
        return part;
    }
    return "";
}
function ownerPropertySegments(fullReceiver) {
    const parts = fullReceiver.split(".").map((part) => part.trim()).filter((part) => part !== "" && part !== "this");
    return parts.slice(1);
}
function classifyOwner(fullReceiver, funcName) {
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
function resolveNep(callee) {
    const method = terminalMethodName(callee);
    const cat = NEP_API_MAP[method];
    return cat ? { isNepAPI: true, nepAPIType: cat } : { isNepAPI: false, nepAPIType: "" };
}
function argTexts(node, src) {
    return node.arguments.map((a) => a.getText(src));
}
function bodyTextOf(node, sourceText) {
    return sourceText.substring(node.getStart(), node.getEnd());
}
function varKind(flags) {
    if (flags & ts.NodeFlags.Const)
        return "const";
    if (flags & ts.NodeFlags.Let)
        return "let";
    return "var";
}
function typeText(typeNode, src) {
    return typeNode ? typeNode.getText(src) : "";
}
function isExported(node) {
    const mods = ts.canHaveModifiers(node) ? ts.getModifiers(node) : undefined;
    return mods?.some((m) => m.kind === ts.SyntaxKind.ExportKeyword) ?? false;
}
function isAsync(node) {
    const mods = ts.canHaveModifiers(node) ? ts.getModifiers(node) : undefined;
    return mods?.some((m) => m.kind === ts.SyntaxKind.AsyncKeyword) ?? false;
}
function firstStringArg(node) {
    const a = node.arguments[0];
    if (a && (ts.isStringLiteral(a) || ts.isNoSubstitutionTemplateLiteral(a)))
        return a.text;
    return "<dynamic>";
}
function callbackBody(node) {
    for (const arg of node.arguments) {
        if (ts.isFunctionExpression(arg) || ts.isArrowFunction(arg))
            return arg.body;
    }
    return undefined;
}
function callbackIsAsync(node) {
    for (const arg of node.arguments) {
        if (ts.isFunctionExpression(arg) || ts.isArrowFunction(arg))
            return isAsync(arg);
    }
    return false;
}
// ── Call Collector ──────────────────────────────────────────────────────────────
function collectCalls(node, src, sourceText) {
    const calls = [];
    const seenBySpan = new Map();
    function walk(n) {
        let target = n;
        let awaited = false;
        if (ts.isAwaitExpression(n) && ts.isCallExpression(n.expression)) {
            target = n.expression;
            awaited = true;
        }
        if (ts.isCallExpression(target)) {
            const callee = calleeText(target, src);
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
                const existing = calls[seenBySpan.get(key)];
                existing.isAwait = existing.isAwait || awaited;
                return;
            }
            calls.push({
                callee,
                args: argTexts(target, src),
                arguments: argTexts(target, src),
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
function buildDescribe(node, src, sourceText) {
    const block = {
        name: firstStringArg(node),
        startLine: lineOf(src, node.getStart()),
        endLine: lineOf(src, node.getEnd()),
        beforeEach: null, afterEach: null, beforeAll: null, afterAll: null,
        tests: [],
        nestedDescribes: [],
    };
    const body = callbackBody(node);
    if (!body)
        return block;
    const stmts = ts.isBlock(body) ? body.statements : [];
    for (const stmt of stmts) {
        if (!ts.isExpressionStatement(stmt))
            continue;
        const expr = stmt.expression;
        if (!ts.isCallExpression(expr))
            continue;
        const callee = calleeText(expr, src);
        const method = terminalMethodName(callee);
        if (method === "describe") {
            block.nestedDescribes.push(buildDescribe(expr, src, sourceText));
        }
        else if (method === "it" || method === "test") {
            const cbBody = callbackBody(expr);
            block.tests.push({
                name: firstStringArg(expr),
                startLine: lineOf(src, expr.getStart()),
                endLine: lineOf(src, expr.getEnd()),
                calls: cbBody ? collectCalls(cbBody, src, sourceText) : [],
                isAsync: callbackIsAsync(expr),
            });
        }
        else if (TEST_HOOKS.has(method)) {
            const hookBody = callbackBody(expr);
            const hookBlock = {
                startLine: lineOf(src, expr.getStart()),
                endLine: lineOf(src, expr.getEnd()),
                bodyText: hookBody ? bodyTextOf(hookBody, sourceText) : "",
            };
            if (method === "beforeEach")
                block.beforeEach = hookBlock;
            else if (method === "afterEach")
                block.afterEach = hookBlock;
            else if (method === "beforeAll")
                block.beforeAll = hookBlock;
            else if (method === "afterAll")
                block.afterAll = hookBlock;
        }
    }
    return block;
}
// ── Imports Extractor ──────────────────────────────────────────────────────────
function extractImport(node, src) {
    const module = node.moduleSpecifier.text;
    const clause = node.importClause;
    const specifiers = [];
    let isDef = false;
    let isNs = false;
    let alias = "";
    if (clause) {
        if (clause.name) {
            isDef = true;
            specifiers.push(clause.name.text);
            alias = clause.name.text;
        }
        const bindings = clause.namedBindings;
        if (bindings) {
            if (ts.isNamespaceImport(bindings)) {
                isNs = true;
                specifiers.push(bindings.name.text);
            }
            else if (ts.isNamedImports(bindings)) {
                for (const el of bindings.elements)
                    specifiers.push(el.name.text);
            }
        }
    }
    const line = lineOf(src, node.getStart());
    const names = specifiers.join(",");
    const isNep = module.toLowerCase().includes("nep");
    return { module, names, alias, line, isNep, specifiers, isDefault: isDef, isNamespace: isNs };
}
// ── Params Extractor ───────────────────────────────────────────────────────────
function extractParams(params, src) {
    return params.map((p) => p.name.getText(src));
}
function flattenDescribeTests(describes, out) {
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
function analyseFile(filePath) {
    const absolutePath = path.resolve(filePath);
    const sourceText = fs.readFileSync(absolutePath, "utf-8");
    const ext = path.extname(filePath).toLowerCase();
    const scriptKind = ext === ".tsx" ? ts.ScriptKind.TSX :
        ext === ".jsx" ? ts.ScriptKind.JSX :
            ext === ".js" ? ts.ScriptKind.JS : ts.ScriptKind.TS;
    const src = ts.createSourceFile(path.basename(filePath), sourceText, ts.ScriptTarget.Latest, true, scriptKind);
    const language = ext === ".js" || ext === ".jsx" ? "javascript" : "typescript";
    const rawLineCount = sourceText.split("\n").length;
    const warnings = [];
    const parseErrors = [];
    if (rawLineCount > 10000)
        warnings.push("large_file");
    // Collect parse diagnostics (ts.createSourceFile stores them internally).
    const diags = src.parseDiagnostics;
    if (diags) {
        for (const d of diags) {
            const msg = ts.flattenDiagnosticMessageText(d.messageText, " ");
            const line = lineOf(src, d.start ?? 0);
            parseErrors.push(`Line ${line}: ${msg}`);
        }
    }
    const imports = [];
    const topLevelVariables = [];
    const functions = [];
    const allCalls = [];
    const describes = [];
    const constants = [];
    const variables = [];
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
                }
                else {
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
                if (decl.initializer &&
                    (ts.isArrowFunction(decl.initializer) || ts.isFunctionExpression(decl.initializer))) {
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
            }
            else if (method === "it" || method === "test") {
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
function main() {
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
    const results = args.map((f) => analyseFile(f));
    const output = args.length === 1 ? results[0] : results;
    process.stdout.write(JSON.stringify(output, null, 2) + "\n");
}
main();
