import { readFileSync } from "node:fs";
import { registerHooks } from "node:module";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const JSX = { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 };

/** Lets `node --test` load the app as Vite does: an extensionless relative import is the `.ts` file, else the `.tsx` one, else the folder's `index.ts`; a `.tsx` file is compiled with TypeScript, so a test can render a component. */
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (!/^\.\.?\//.test(specifier) || /\.[cm]?[jt]sx?$/.test(specifier)) return nextResolve(specifier, context);
    for (const suffix of [".ts", ".tsx"]) {
      try {
        return nextResolve(`${specifier}${suffix}`, context);
      } catch {
        continue;
      }
    }
    return nextResolve(`${specifier}/index.ts`, context);
  },
  load(url, context, nextLoad) {
    if (!url.endsWith(".tsx")) return nextLoad(url, context);
    const source = ts.transpileModule(readFileSync(fileURLToPath(url), "utf8"), { compilerOptions: JSX, fileName: fileURLToPath(url) }).outputText;
    return { format: "module", source, shortCircuit: true };
  },
});
