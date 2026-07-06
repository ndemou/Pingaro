# Agent Notes

- `pingaroui.exe` is the native Windows graphical app. Always build it with `go build -ldflags="-H=windowsgui" -o pingaroui.exe ./cmd/pingaroui` so launching it does not open or depend on a terminal window.
