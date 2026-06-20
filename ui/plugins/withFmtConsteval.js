const { withDangerousMod } = require('@expo/config-plugins');
const fs = require('fs');
const path = require('path');

// Xcode 16+/26 ship a clang that rejects fmt's FMT_STRING consteval path
// ("call to consteval function ... is not a constant expression"), which breaks
// the `fmt` pod that React Native's RCT-Folly depends on. Defining
// FMT_USE_CONSTEVAL=0 makes fmt fall back to constexpr and compiles cleanly.
const MARKER = 'FMT_USE_CONSTEVAL=0';
const SNIPPET = `
  # Injected by plugins/withFmtConsteval.js — fix fmt/RCT-Folly build under Xcode 16+/26.
  installer.pods_project.targets.each do |fmt_target|
    fmt_target.build_configurations.each do |fmt_config|
      fmt_config.build_settings['GCC_PREPROCESSOR_DEFINITIONS'] ||= ['$(inherited)']
      fmt_config.build_settings['GCC_PREPROCESSOR_DEFINITIONS'] += ['${MARKER}']
    end
  end
`;

module.exports = function withFmtConsteval(config) {
  return withDangerousMod(config, [
    'ios',
    (config) => {
      const podfile = path.join(config.modRequest.platformProjectRoot, 'Podfile');
      let contents = fs.readFileSync(podfile, 'utf8');
      if (!contents.includes(MARKER)) {
        contents = contents.replace(/post_install do \|installer\|/, `post_install do |installer|\n${SNIPPET}`);
        fs.writeFileSync(podfile, contents);
      }
      return config;
    },
  ]);
};
