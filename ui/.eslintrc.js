module.exports = {
  extends: 'expo',
  ignorePatterns: ['dist/*', 'src/api/generated/*'],
  rules: {
    // ponytail: eslint-config-expo (SDK 56) bumped eslint-plugin-react-hooks to a
    // version whose "recommended" preset now includes the React Compiler
    // readiness rules. They flag ~20 pre-existing, working call sites (sync
    // setState-from-query-data in effects, ref reads in effect deps, Date.now()
    // in render). Downgrade to warn until those are audited and fixed one by one;
    // don't silently rewrite hook logic as a side effect of an SDK bump.
    'react-hooks/set-state-in-effect': 'warn',
    'react-hooks/refs': 'warn',
    'react-hooks/purity': 'warn',
  },
};
