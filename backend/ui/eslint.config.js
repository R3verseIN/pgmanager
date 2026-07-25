import js from '@eslint/js';
import globals from 'globals';
import tailwind from 'eslint-plugin-tailwindcss';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist'] },
  {
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommended,
    ],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    plugins: {
      tailwindcss: tailwind,
    },
    rules: {
      ...tailwind.configs.recommended.rules,
      "tailwindcss/no-custom-classname": ["warn", {
        whitelist: [
          "animate-.*", "fade-.*", "zoom-.*", "slide-.*",
          "fill-mode-.*", "data-\\[state.*", "data-\\[side.*", "inputs"
        ]
      }],
      "tailwindcss/no-unnecessary-arbitrary-value": "off",
    },
    settings: {
      tailwindcss: {
        callees: ['cn', 'cva'],
        config: 'src/styles.css',
      }
    }
  }
);
