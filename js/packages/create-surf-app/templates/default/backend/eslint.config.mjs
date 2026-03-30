import js from '@eslint/js';
import globals from 'globals';

export default [
  { ignores: ['node_modules', 'lib/api*.js'] },
  {
    files: ['**/*.js'],
    ...js.configs.recommended,
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'commonjs',
      globals: globals.node,
    },
    rules: {
      'no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      'no-empty': 'warn',
      'eqeqeq': 'warn',
      'no-fallthrough': 'warn',
    },
  },
];
