import { registerHooks } from "node:module";

/** Lets `node --test` load the app's extensionless relative imports as Vite does: the `.ts` file, else the folder's `index.ts`. */
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (!/^\.\.?\//.test(specifier) || /\.[cm]?[jt]sx?$/.test(specifier)) return nextResolve(specifier, context);
    try {
      return nextResolve(`${specifier}.ts`, context);
    } catch {
      return nextResolve(`${specifier}/index.ts`, context);
    }
  },
});
