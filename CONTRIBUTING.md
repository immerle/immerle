# Contributing to Immerle

Thanks for helping Immerle sing! 🐦 Issues and pull requests are very welcome.

## Reporting issues

- **Bugs & features** — open an issue using the templates.
- **Security vulnerabilities** — do *not* open a public issue; see
  [SECURITY.md](SECURITY.md).

## Development loop

You'll need **Go 1.25+** and `ffmpeg`/`ffprobe` on your `PATH`.

```bash
make test        # run the suite
make test-race   # with the race detector
make lint        # golangci-lint
make ci          # tidy + vet + lint + test + build
```

Tests that need real audio generate fixtures with `ffmpeg` and skip when it is
not installed.

## Before opening a PR

1. Run `make ci` — it must pass.
2. If you touched handler annotations, regenerate the OpenAPI spec with
   `make openapi` — CI fails on a stale spec.
3. Keep commits focused; follow the existing
   [Conventional Commits](https://www.conventionalcommits.org/) style you'll
   see in `git log` (e.g. `feat(api): ...`, `fix(desktop): ...`).

See [Architecture & development](https://immerle.com/developers/architecture) for the full reference and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community expectations.

By contributing, you agree your contributions are licensed under the project's
[AGPLv3](LICENSE).
