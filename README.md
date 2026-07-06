## Pingaro

Long term network quality monitor

Pingaro is a native Windows desktop app for watching connection quality over time. It visualizes response time, packet loss, and jitter in real time and across longer aggregate windows.

<img width="1094" height="926" alt="image" src="https://github.com/user-attachments/assets/f5136898-dfc7-461a-9123-9e4aefb93e33" />

## Download From GitHub

1. Open the Pingaro releases page: <https://github.com/ndemou/Pingaro/releases>
2. Open the latest release.
3. Download `pingaro.exe` from the release assets.
4. In PowerShell, go to the download folder and run:

```powershell
.\pingaro.exe
```

If Windows shows a security warning because the executable is unsigned, choose `More info` and then `Run anyway` only if you downloaded it from the GitHub release page above.

## Why Use It

Raw `ping.exe` output is hard to interpret over more than a few seconds. Pingaro gives you a dashboard view of the same kind of signal: current RTT, aggregated p95 RTT, packet loss, and one-way jitter estimates.

When checking Internet quality, Pingaro can ping multiple well-known hosts in parallel. If at least one host replies in a batch, the batch is treated as successful and the minimum RTT is used. That makes the view less sensitive to a temporary issue on one remote host.

## Typical Workflow

1. Start `pingaro.exe`.
2. Leave the default Gateway and Internet targets, or enter one to three target groups.
3. Select one or more Internet uses.
4. Click `Start`.
5. Watch the graphs for spikes, packet loss, or sustained jitter.

By default Pingaro selects `email & browsing`, `audio calls`, `video calls`, and `online gaming`. It leaves `remote desktop` and `Superhuman Gaming` unchecked.

When multiple uses are selected, Pingaro grades good, medium, and bad measurements using the strictest threshold from the selected uses. The jitter graph is shown only when `audio calls` or `video calls` is selected.

## Metrics

**RTT** is round-trip time in milliseconds.

**p95 RTT** is the 95th percentile RTT for an aggregate period. It shows high latency that affects normal use without letting a single extreme outlier dominate the graph.

**Loss** is the percent of ping batches with no reply.

**One-way jitter** is estimated as half the two-way jitter.

## Arguments

Pingaro is configured through the desktop UI. The saved config is reused on the next launch.

## Build

```powershell
go generate ./...
go build -trimpath -ldflags="-H=windowsgui" -o pingaro.exe .
```

The `-H=windowsgui` linker flag is required. Without it, Windows starts the app through a console subsystem and opens a terminal window that must remain open.
`go generate ./...` refreshes `rsrc.syso` from `pingaro.exe.manifest` and `assets/pingaro.ico`.

## Run

```powershell
.\pingaro.exe
```

Pingaro stores settings and saved history under `%AppData%\Pingaro`.
