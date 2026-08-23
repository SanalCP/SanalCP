import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import jsxA11y from 'eslint-plugin-jsx-a11y'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', 'node_modules'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended, jsxA11y.flatConfigs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      // tsconfig noUnusedLocals/noUnusedParameters kasıtlı kapalı (geliştirme kolaylığı) —
      // lint tarafında da aynı gevşeklik korunur, ikisi çelişmesin.
      '@typescript-eslint/no-unused-vars': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      // React Compiler hazırlığı için eklenen agresif kurallar — bu projede React 18
      // kullanılıyor (Compiler yok). "useEffect(() => api.get().then(setX), [])" standart
      // data-fetching pattern'i kod tabanı genelinde onlarca sayfada kullanılıyor; "en son
      // callback'i ref'te tut" (Modal.tsx) da yaygın, güvenli bir pattern. İkisini de hata
      // saymak gerçek bir bug'ı değil, idiyomatik kodu işaret ediyor.
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/refs': 'off',
      'react-hooks/immutability': 'off',
      // Her iki dalı da (yalnız) yan-etkili bir çağrı olan ternary'ler — "if/else yerine
      // kısa ternary" bu kod tabanında yaygın bir stil, davranışta belirsizlik yok.
      '@typescript-eslint/no-unused-expressions': ['error', { allowTernary: true }],
      // Boş catch: bu kod tabanında KASITLI sessiz yutma (ör. clipboard fallback zinciri) —
      // yorum eklemek zorunlamak yerine izin ver.
      'no-empty': ['error', { allowEmptyCatch: true }],
      // autoFocus kullanımlarının TÜMÜ modal/dialog açılışı veya login formunda ilk input'a
      // focus — WAI-ARIA Authoring Practices dialog pattern'inin standart önerisi budur
      // ("dialog açılınca odak ilk odaklanabilir elemana gider"). Kaldırmak gerçek bir UX
      // regresyonu olurdu (kullanıcı artık manuel tıklamalı); kural burada fazla genel.
      'jsx-a11y/no-autofocus': 'off',
    },
  },
)
