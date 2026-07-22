<img width="427" height="316" alt="image" src="https://github.com/user-attachments/assets/0b9dab51-f80e-4157-81bd-75f1dac9956c" />

Pingaro is a Windows app for monitoring connection quality over time. If you are a network expert, it gives you
clear visualizations of response time, packet loss, and jitter. If you are not, but just want to know whether your
Internet connection is good enough, pick a use profile, such as Audio Calls or Online Gaming, and let Pingaro highlight
any quality issues it detects. You can leave it running in the background and review the results later.

<img width="1226" height="943" alt="image" src="https://github.com/user-attachments/assets/a7b7c7a8-9c4e-41f8-a11c-214a38f0dfc2" />

## Download & Use

1. Open the Pingaro releases page: <https://github.com/ndemou/Pingaro/releases>
2. Download the top most `pingaro.exe` from the release assets and run it (no installation is needed).

   <img width="192" height="122" alt="image" src="https://github.com/user-attachments/assets/981d1775-dc20-41a2-b90b-b3998ab232ec" />

***If** Windows shows a security warning* because the executable is unsigned, choose `More info` and then `Run anyway` (only if you downloaded it from the GitHub release page above).

3. Based on your Internet usage you may want to check/uncheck one or more use profiles.

<img width="290" height="204" alt="image" src="https://github.com/user-attachments/assets/c6287083-b70c-4519-97e0-c69349575b80" />

4. **If** *you made any changes*, click `Stop` and then `Start`.
5. Start using your Internet connection normaly and from time to time, take a look at the graphs for any grayish, yellowish or redish markers:

<img width="374" height="191" alt="image" src="https://github.com/user-attachments/assets/f5ffb518-b98c-4124-82fb-54996b04a1e0" />

Or just read the Quality Assesment at the bottom left corner:

<img width="401" height="197" alt="image" src="https://github.com/user-attachments/assets/6a82a2ac-8bc0-4094-afe2-82b63a0e1e82" />

## Why Use It

If you are not an expert pingaro will easily answer this question: "is my Internet connection good enough for my use case?" (it will not answer the unrelated question "is my Internet connection fast enough").

If you are an expert take a look at the screenshot and you know 90% of what you are getting: a dashboard view of connection quality over time: live latency, aggregated p95 RTT, packet loss, and one-way jitter estimates. Also note that when checking Internet quality (or manually entered lists of hosts), Pingaro measures multiple hosts in parallel. If at least one host replies in a batch, the batch is treated as successful and the minimum RTT is used. That makes the view less sensitive to a temporary issue on one remote host.

Settings are auto-saved in `%AppData%\Pingaro\settings.json`. 

Everything displayed is also automaticly saved under `%AppData%\Pingaro` as timestamped files such as `history-2026-07-09_08.52.31.json`. 

Measurements are scheduled at a steady cadence. Replies that arrive too late are treated as lost, so the live graph reflects delayed or missing responses promptly instead of waiting for old replies to catch up.

You can include more than one hosts per target group. A ping will be considered succesful if **any** of the hosts reply and the minimum RTT is recorded. You should prefer to include at least two targets because every once in a while a host may fail to respond. For example, to check the quality of your WiFi, add both your default gateway, and the IP of a PC that is *wired* to your LAN.

The special target name `gateway` resolves to the current default gateway IP address, `internet` is resolved to 4 well known IPs, and `localhost` resolves to `127.0.0.1`.

When multiple use profiles are selected, Pingaro grades good, medium, and bad measurements using the strictest threshold from the selected profiles. 

### Metrics

**RTT** is round-trip time in milliseconds.

**p95 RTT** is the 95th percentile RTT for an aggregate period. It shows high latency that affects normal use without letting a single extreme outlier dominate the graph.

**Loss** is the percent of measurement batches with no on-time reply.

**One-way jitter** is estimated as half the measured two-way jitter.

### Files

Pingaro stores settings, and manual history exports under `%AppData%\Pingaro`. It also automaticly saves timestamped history files at the same folder.

## Build

```powershell
go generate ./...
go build -trimpath -ldflags="-H=windowsgui" -o pingaro.exe .
```

Release builds should also set the visible version and build time:

```powershell
$buildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:sszzz"
go build -trimpath -ldflags="-H=windowsgui -X main.appVersion=vX.Y.Z -X main.appBuildTime=$buildTime" -o pingaro.exe .
```
