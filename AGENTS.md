# Agent Notes

- Pingaro has only one application binary: `pingaro.exe`. Always build it with `go build -trimpath -ldflags="-H=windowsgui" -o pingaro.exe .` so launching it does not open or depend on a terminal window.
- Run `go generate ./...` before release builds when the Windows manifest or `assets/pingaro.ico` changes; it refreshes `rsrc.syso`, which embeds the manifest and executable icon.
