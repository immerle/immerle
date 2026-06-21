const { withDangerousMod } = require('@expo/config-plugins');
const fs = require('fs');
const path = require('path');

// fmt 11 (vendored by React Native's RCT-Folly) hard-codes FMT_USE_CONSTEVAL=1
// for modern clang in base.h — with no #ifndef guard, so a -D override is
// ignored. Xcode 16+/26's stricter clang then rejects fmt's FMT_STRING consteval
// path ("call to consteval function ... is not a constant expression"), breaking
// the build. We patch base.h in the Podfile post_install to force consteval off
// (constexpr), which compiles cleanly. The hook re-runs on every pod install, so
// it survives a fresh `expo prebuild`.
const MARKER = 'withFmtConsteval';
const SNIPPET = `
  # Injected by plugins/withFmtConsteval.js — disable fmt consteval (Xcode 16+/26).
  fmt_base = File.join(installer.sandbox.root, 'fmt', 'include', 'fmt', 'base.h')
  if File.exist?(fmt_base)
    fmt_text = File.read(fmt_base)
    fmt_patched = fmt_text.gsub('#  define FMT_USE_CONSTEVAL 1', '#  define FMT_USE_CONSTEVAL 0')
    File.write(fmt_base, fmt_patched) if fmt_patched != fmt_text
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
