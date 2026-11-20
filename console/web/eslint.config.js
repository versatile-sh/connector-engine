import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist/**"] },
  js.configs.recommended,
  tseslint.configs.recommended,
  reactHooks.configs.flat.recommended,
  { languageOptions: { globals: globals.browser } },
  { files: ["src/activity/HourlyChart.tsx"], rules: { "react-hooks/immutability": "off" } },
  { files: ["src/skills/SkillsPage.tsx"], rules: { "react-hooks/refs": "off" } },
);
