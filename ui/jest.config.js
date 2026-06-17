/** Jest config. Pure-logic units (md5, formatters, capability merge) run on the
 * node environment via ts-jest-free babel transform from jest-expo. */
module.exports = {
  preset: 'jest-expo',
  testMatch: ['**/*.test.ts', '**/*.test.tsx'],
  transformIgnorePatterns: [
    'node_modules/(?!((jest-)?react-native|@react-native(-community)?|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|react-navigation|@react-navigation/.*|@unimodules/.*|unimodules|sentry-expo|native-base|nativewind|react-native-css-interop))',
  ],
};
