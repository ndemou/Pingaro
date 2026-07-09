# Agent Notes

- Pingaro has only one application binary: `pingaro.exe`. Always build it with `go build -trimpath -ldflags="-H=windowsgui" -o pingaro.exe .` so launching it does not open or depend on a terminal window.
- Run `go generate ./...` before release builds when the Windows manifest or `assets/pingaro.ico` changes; it refreshes `rsrc.syso`, which embeds the manifest and executable icon.
- Keep `docs/design-decisions.html` current. Any change to product scope, naming, UX, target behavior, metrics, thresholds, colors, defaults, persistence, build/release process, or other intentional design choices must update that document in the same change. If no update is needed, state that explicitly in the final response.
- Rich visual styling is welcome when it supports the tone and readability of the design document, but structural complexity must serve understanding. Use color, gradients, and shadows to create a polished, pleasant reading experience; use complex HTML layouts or components only when they clarify structure, comparison, hierarchy, color meaning, or a UI relationship.
