module.exports = function (api) {
  api.cache(true);
  return {
    // babel-preset-expo auto-adds `react-native-reanimated/plugin`, and
    // `nativewind/babel` re-applies it last (reanimated requires it be last),
    // so we do NOT list it explicitly here to avoid registering it twice.
    presets: [
      ['babel-preset-expo', { jsxImportSource: 'nativewind' }],
      'nativewind/babel',
    ],
  };
};
