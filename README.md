<img width="427" height="316" alt="image" src="https://github.com/user-attachments/assets/0b9dab51-f80e-4157-81bd-75f1dac9956c" />

Pingaro is a Windows app for monitoring connection quality over time. If you are a network expert, it gives you
clear visualizations of response time, packet loss, and jitter. If you are not, but just want to know whether your
Internet connection is good enough, pick a use profile, such as Audio Calls or Online Gaming, and let Pingaro highlight
any quality issues it detects. You can leave it running in the background and review the results later.

<img width="1211" height="948" alt="image" src="https://github.com/user-attachments/assets/1ab3a613-a6ea-463e-a2a6-641fc815b2ec" />

Settings are saved in `%AppData%\Pingaro\settings.json`. Automatic capture history is saved under `%AppData%\Pingaro` as timestamped files such as `history-2026-07-09_08.52.31.json`. Manual history Save/Load defaults to `history.json`. Older versions used `%AppData%\Pingaro\pingaro.json` for settings and `%AppData%\Pingaro\pingaro-history.json` for history; Pingaro migrates those legacy files to the clearer names when needed.

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

Raw `ping.exe` output is hard to interpret over more than a few seconds. Pingaro gives you a dashboard view of the same kind of signal: live latency, aggregated p95 RTT, packet loss, and one-way jitter estimates.

When checking Internet quality, Pingaro can ping multiple well-known hosts in parallel. If at least one host replies in a batch, the batch is treated as successful and the minimum RTT is used. That makes the view less sensitive to a temporary issue on one remote host.

## Typical Workflow

1. Start `pingaro.exe`.
2. Leave the default Gateway and Internet targets, or enter one to three target groups.
3. Select one or more use profiles.
4. Click `Start`.
5. Watch the graphs for spikes, packet loss, or sustained jitter.

The special target name `localhost` resolves to `127.0.0.1`. The special target name `gateway` resolves to the current default gateway IP address.

By default Pingaro selects `Browsing & Email`, `Audio Calls`, `Video Calls`, and `Online Gaming`. It leaves `Remote Desktop` and `Superhuman Gaming` unchecked.

When multiple use profiles are selected, Pingaro grades good, medium, and bad measurements using the strictest threshold from the selected profiles. The jitter graph is shown only when `Audio Calls` or `Video Calls` is selected.

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

Pingaro stores settings, manual history exports, and automatic timestamped history files under `%AppData%\Pingaro`.
