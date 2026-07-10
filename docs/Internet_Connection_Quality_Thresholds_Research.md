# Internet Connection Quality Thresholds Research

## Part 1 

### 1. Engineering / Planning Thresholds

These represent the network conditions required for users to perceive the experience as genuinely **good**.

They answer questions such as:

- *Will this Teams call feel natural?*
- *Will gaming feel responsive?*
- *Will Remote Desktop feel local?*

These thresholds are typically quite strict.

---

### 2. Operational / Telemetry Thresholds

These represent the conditions under which an application is still considered **operational**.

Modern applications can hide a surprising amount of network impairment using:

- adaptive codecs,
- jitter buffers,
- packet concealment,
- retransmissions,
- adaptive bitrate,
- forward error correction.

Consequently, many vendors use much more relaxed thresholds in their monitoring tools.

For example:

- Microsoft Teams [1]' **network connectivity test** recommends approximately:
  - **Latency <100 ms**
  - **Jitter <30 ms**
  - **Packet loss <1%**

Yet Microsoft's **real-time meeting telemetry** considers a meeting "good" with approximately:

- **RTT <500 ms**
- **Jitter <30 ms**
- **Packet loss <5%**

These are diagnostic thresholds indicating that the meeting is still functioning—not that users are experiencing excellent quality.

---

## What this means for your classifier

If your software is intended to answer:

> **"Will the user perceive this Internet connection as Good, Medium or Bad for this workload?"**

then the thresholds should be much closer to the **engineering/planning** values than the operational telemetry values.

If, instead, the purpose is:

> **"Will the application still function?"**

then the much looser vendor telemetry thresholds are appropriate.

For a user-facing connection quality tool, the research strongly supports using the first approach: classify according to **expected user experience**, not merely whether the application continues to operate.

---

## Part 2 — Standards and Vendor Recommendations

One of the most interesting findings of this research is that there is **no single universally accepted set of thresholds** for latency, packet loss, and jitter. Instead, there are three different sources of guidance:

1. **Standards organizations** (ITU-T, IETF)
2. **Application vendors** (Microsoft, Google, Zoom [3], Cisco, NVIDIA, etc.)
3. **Academic research on user perception and performance**

They all agree on the fundamentals, but they optimize for different goals.

---

## 1. ITU Standards

The ITU recommendations [2] remain the best high-level engineering guidance.

### ITU-T G.1010

This recommendation classifies applications according to the network quality they require.

### Conversational Voice

Preferred:

- **One-way delay <150 ms**
- **Packet loss <3%**

Maximum acceptable:

- **One-way delay <400 ms**

Beyond approximately 400 ms, normal conversation becomes difficult because people begin talking over each other.

---

### Video Conferencing

Preferred:

- **One-way delay <150 ms**
- **Packet loss approximately <1%**

Maximum:

- **One-way delay <400 ms**

Video codecs can conceal packet loss better than audio, but interactive conversation still suffers from excessive delay.

---

### Interactive Games

The recommendation gives

- **One-way delay <200 ms**

and effectively assumes **no application-level information loss**.

Notice that the standard says **200 ms one-way**, not RTT. That corresponds to roughly **400 ms RTT**, which is much higher than what modern gamers would consider acceptable.

This reflects the age of the recommendation rather than current player expectations.

---

### Web Browsing and Email

Interestingly, G.1010 does **not** specify latency or jitter limits.

Instead it specifies **response time**:

Preferred:

- **<2 seconds**

Acceptable:

- **<4 seconds**

This is a very important distinction.

For browsing, users care about

- page load time,
- click responsiveness,
- transaction completion,

not packet arrival timing.

This strongly supports the idea that **jitter is not a primary metric for Browsing & Email**.

---

## ITU-T G.114

G.114 is one of the classic references for voice latency.

It states that

- **One-way delay below approximately 150 ms is essentially transparent**
- **Above 150 ms conversation quality gradually deteriorates**
- **400 ms is the practical upper planning limit**

These numbers have survived for decades because they closely match human conversational behavior.

---

## ITU-T Y.1541

Y.1541 defines IP Quality-of-Service classes.

For the strictest real-time service classes it recommends approximately:

- **100 ms mean one-way delay**
- **50 ms IP Packet Delay Variation (IPDV)**
- **Packet loss probability of 10⁻³**

This recommendation is aimed at **network engineering**, not consumer Internet connections.

---

# 2. Vendor Recommendations

Modern application vendors generally recommend **stricter thresholds** than the ITU standards when describing what users perceive as *good quality*.

---

## Zoom [3]

Zoom [3] recommends approximately:

- **Latency ≤150 ms**
- **Jitter ≤40 ms**
- **Packet loss ≤2%**

Above these values users begin to notice:

- delayed conversations
- frozen video
- robotic audio

These values align closely with real-world experience.

---

## Google Meet

Google recommends:

- **RTT to Google <100 ms** for best quality

It also notes that

- media quality noticeably degrades around **300 ms RTT**.

---

## Google Voice [7]

Google recommends

- **RTT below 100 ms**
- **Average jitter below 30 ms**
- **Packet loss as close to zero as possible**

Notice how aggressively Google treats packet loss.

---

## Microsoft Teams [1]

Microsoft actually publishes **two different sets of thresholds**.

### Network Connectivity Test

This tool checks whether the network is capable of delivering high-quality calls.

Typical recommendations are

- **UDP latency <100 ms**
- **Jitter <30 ms**
- **Packet loss <1%**

---

### Real-Time Meeting Telemetry

The Teams admin portal uses much looser thresholds:

- **RTT <500 ms**
- **Jitter <30 ms**
- **Packet loss <5%**

These are **diagnostic thresholds**, not quality targets.

They indicate that the meeting is still functioning, not that users are having an excellent experience.

This distinction explains why Microsoft's published numbers sometimes appear contradictory.

---

## Remote Desktop

Microsoft describes Azure Virtual Desktop [4] as a **real-time, latency-sensitive workload**.

Their guidance is roughly:

- **≤150 ms**: little impact for office work
- **150–200 ms**: generally acceptable for text-oriented tasks
- **>200 ms**: responsiveness begins to degrade noticeably

Microsoft also emphasizes that:

- added latency,
- added jitter,
- TLS inspection,
- VPN detours,

all negatively affect the desktop experience.

---

## Citrix

Citrix's documentation distinguishes between

- **survivability**
- **quality**

For example, adaptive graphics modes may activate at approximately:

- **300 ms latency**
- **5% packet loss**

These are **not** recommendations for good user experience.

They are thresholds at which Citrix starts using increasingly aggressive techniques to keep the session usable despite poor network conditions.

---

# 3. Gaming

Gaming guidance naturally splits into two categories:

## Traditional Online Gaming

EA [5] identifies exactly three primary metrics:

- Ping
- Jitter
- Packet Loss

EA [5] explicitly states that packet loss should be **as close to 0% as possible**, and considers all three metrics essential for connection quality.

---

## Cloud Gaming

Cloud gaming has much stricter requirements.

### NVIDIA GeForce NOW [11]

NVIDIA recommends approximately:

- **Latency <20 ms** for excellent experience

and places strong emphasis on

- latency,
- jitter,
- packet loss.

---

### Xbox Cloud Gaming

Xbox recommends approximately:

- **Latency <80 ms**
- **Jitter <20 ms**

For Remote Play, Microsoft states:

- **<150 ms** is required
- **<60 ms** is optimal

---

## Academic Research

Recent gaming research consistently shows:

- measurable improvements below **40 ms**
- additional benefits between **12–20 ms** for competitive FPS games

However, these studies typically measure **end-to-end system latency**, not just Internet RTT.

This means your "Superhuman Gaming" profile makes sense as an **elite / esports tier**, but it should not be presented as a baseline expectation for ordinary Internet connections.

---

# Part 3 — Recommended Thresholds (Including the Table)

After reviewing the ITU recommendations [2], Microsoft, Google, Zoom [3], NVIDIA, Citrix, EA [5], Cloudflare [12] guidance, and the networking literature, the following thresholds provide a better match to **actual user-perceived quality** rather than merely application survivability.

The philosophy behind these recommendations is:

- **Good** = Most users would consider the experience smooth and responsive.
- **Medium** = Clearly usable, but users will occasionally notice delays or quality degradation.
- **Bad** = The application still works in many cases, but the network has become the dominant factor limiting usability.

---

## Recommended Thresholds

| Profile           | RTT (ms) Good / Medium / Bad | Packet Loss (%) Good / Medium / Bad | Jitter (ms) Good / Medium / Bad |
|-------------------|------------------------------:|------------------------------------:|--------------------------------:|
| Browsing & Email  | **100 / 200 / 500** | **0.5 / 2 / 5**   | **Not recommended** |
| Remote Desktop    | **80 / 150 / 250**  | **0.5 / 2 / 5**   | **20 / 40 / 80**    |
| Audio Calls       | **100 / 150 / 250** | **0.5 / 1 / 3**   | **15 / 30 / 60**    |
| Video Calls       | **100 / 150 / 250** | **1 / 2 / 5**     | **20 / 40 / 80**    |
| Online Gaming     | **35 / 60 / 120**   | **0.2 / 0.5 / 1** | **10 / 20 / 40**    |
| Superhuman Gaming | **15 / 30 / 60**    | **0.1 / 0.5 / 1** | **5 / 10 / 20**     |

---

## Rationale for the Changes

Several consistent patterns emerged across all the sources.

### 1. Browsing & Email

This profile changed the most.

Instead of measuring jitter, the recommendation is to evaluate:

- RTT
- Packet loss
- Loaded latency (bufferbloat)

Jitter contributes very little to perceived browsing performance.

The proposed RTT values are significantly stricter because modern websites perform many sequential network operations. An RTT that feels acceptable in isolation can quickly translate into noticeable delays when multiplied across dozens of requests.

---

### 2. Remote Desktop

The recommended thresholds are based on the point where users begin to perceive the remote desktop as "remote."

Below approximately **80–100 ms RTT**, many office tasks feel nearly local.

Between **100 and 150 ms**, the experience remains comfortable, although subtle delays become noticeable.

Beyond **200–250 ms**, most users clearly perceive lag while typing, scrolling, or moving windows.

The jitter thresholds are also tightened because modern RDP implementations increasingly rely on UDP transport and continuous graphics updates.

---

### 3. Audio and Video Calls

The recommended values closely match what Microsoft, Google, and Zoom [3] describe as **high-quality communication**, rather than the looser thresholds used by monitoring systems.

The key insight is that users notice **conversation delay** sooner than codec artifacts.

For this reason, RTT is weighted more heavily than packet loss.

A small amount of packet loss can often be concealed, whereas conversational delay fundamentally changes the interaction between speakers.

---

### 4. Online Gaming

Setting the "Good" RTT threshold to 35 ms is not about network capability—it reflects the reality that many competitive players can distinguish responsiveness improvements even within this range.

Similarly, tightening packet loss and jitter thresholds reflects how sensitive modern prediction and interpolation algorithms are to instability.

---

### 5. Superhuman Gaming

This profile represents an **elite-performance target**, intended for esports or highly competitive players.

The recommended thresholds are deliberately aggressive.

They should **not** be interpreted as requirements for ordinary gaming.

Instead, they answer a different question:

> *"Is this connection among the very best currently achievable over the public Internet?"*

Replacing a strict **0% packet loss** requirement with **0.1%** makes the classifier substantially more robust.

Perfectly loss-free Internet connections are uncommon over long measurement periods, and a single lost packet should not prevent an otherwise excellent connection from being classified as elite.

---

# How Conservative Are These Recommendations?

These thresholds intentionally prioritize **user perception** over **technical survivability**.

That means they are generally **stricter** than the limits used by vendors in dashboards or health reports.

For example:

- A Teams meeting may continue functioning with **5% packet loss**, but few users would describe the experience as *good*.
- A game may remain technically playable at **120 ms RTT**, but competitive players would immediately notice the disadvantage.
- A remote desktop session can survive **5% packet loss**, yet users will already experience delayed typing and uneven cursor movement.

In other words, the recommended thresholds answer:

> **"How good does the connection feel?"**

rather than:

> **"Can the application still operate?"**

This distinction is especially important if your software is intended to rate the quality of an Internet connection from the user's perspective rather than simply detect catastrophic failures.

---

# Part 4 — Browsing & Email Analysis

Browsing and email are fundamentally different from the other usage profiles in one important respect:

**They are transaction-oriented rather than stream-oriented.**

A voice call, video conference, or online game depends on a continuous stream of packets arriving at regular intervals. In contrast, web browsing consists of discrete requests followed by responses. This difference has major implications for which network metrics matter.

---

## What Actually Determines the User Experience?

When a user clicks a link, opens a web page, or downloads an email, the perceived responsiveness depends primarily on:

- DNS lookup time
- TCP or QUIC connection establishment
- TLS handshake
- HTTP request/response latency
- Total page download time
- Browser rendering time

Each of these stages is affected by network latency, but **not all are affected equally by jitter**.

A packet arriving 10 ms earlier or later than the previous packet is almost imperceptible during a web transaction. What users notice is how long it takes before the page begins to load and how quickly it completes.

This is why browser developers, CDNs, and Internet performance researchers focus on **latency and completion time**, not packet delay variation.

---

# RTT Is the Dominant Metric

For browsing, RTT is by far the most influential network metric.

Modern websites are surprisingly "chatty." Even a relatively simple page may involve:

- DNS queries
- HTTPS negotiation
- Multiple API requests
- JavaScript downloads
- CSS downloads
- Font retrieval
- Image loading
- Analytics requests
- Advertisement networks
- Third-party services

Although modern protocols such as HTTP/2 and HTTP/3 reduce the number of round trips, page loading still involves many latency-sensitive operations.

This means that an RTT increase from:

- 20 ms to 100 ms

often has little impact,

whereas an increase from:

- 100 ms to 250 ms

can noticeably slow page loading.

By the time RTT approaches **500 ms**, browsing begins to feel sluggish even if bandwidth is excellent.

---

# Packet Loss

Packet loss affects browsing more than many people realize.

Unlike real-time media, most web traffic uses TCP (or QUIC, which implements similar congestion-control principles).

When packets are lost:

- retransmissions occur,
- congestion windows shrink,
- throughput decreases,
- page completion time increases.

Even relatively small packet-loss rates—around **1–2%**—can significantly slow page loading.

Importantly, browsing generally **does not degrade gracefully**.

Instead of gradually lowering image quality (as video does), pages simply take longer to load or appear to pause unexpectedly.

This makes packet loss a more important factor than many threshold tables suggest.

---

# Why Jitter Hardly Matters

Jitter measures the variation in packet arrival times.

This is crucial for real-time media because packets must be played back at a constant rate.

For browsing, however, the browser simply waits until enough data has arrived.

A page that arrives:

- perfectly evenly,
- or in small bursts,

will usually render identically.

The user notices the **overall completion time**, not the spacing between packets.

Consequently, jitter has very little direct influence on perceived browsing performance unless it is so extreme that it begins to resemble intermittent packet loss.

---

# Bufferbloat Is Much More Important

A far better metric than jitter for browsing is **loaded latency**, often referred to as **bufferbloat**.

Bufferbloat occurs when large queues build up inside network equipment during uploads or downloads.

For example:

A connection may have:

- Idle RTT: **20 ms**

but while someone uploads files or runs a cloud backup:

- Loaded RTT: **300 ms**

In this situation:

- browsing becomes sluggish,
- pages hesitate,
- searches take longer,
- clicking links feels delayed,

even though idle ping remains excellent.

This behavior is far more representative of the user experience than measuring jitter.

Cloudflare [12]'s Internet Quality Score and other modern Internet testing tools increasingly emphasize loaded latency for exactly this reason.

---

# Email

Email behaves similarly to web browsing but is even less sensitive to jitter.

Typical email operations include:

- synchronizing folders,
- downloading messages,
- uploading attachments,
- sending mail.

These are transactional exchanges.

Users notice:

- delays before synchronization,
- slower attachment uploads,
- slower message delivery,

but not packet timing variation.

Again, RTT and packet loss dominate.

---

# Recommended Metrics for This Profile

If the goal is to classify the quality of an Internet connection for Browsing & Email, the research suggests the following order of importance:

1. **RTT**
2. **Packet loss**
3. **Loaded latency (bufferbloat)**
4. **Bandwidth** (only for very large transfers)

Jitter is a distant fifth and generally does not justify being part of the quality classifier.

---

# Implications for Your Program

The research supports changing the Browsing & Email profile in two ways:

1. **Tighten the RTT and packet-loss thresholds** to better reflect when users begin to perceive sluggishness.

2. **Replace jitter with a measure of loaded latency (bufferbloat) [1][12]** if practical.

If measuring loaded latency is not feasible, omitting jitter entirely for this profile is preferable to assigning it significant weight.

This would also make your classifier more closely aligned with both user perception and current industry practice.

---

# Part 5 — Remote Desktop Analysis

Remote Desktop occupies an interesting middle ground between transactional applications (such as web browsing) and continuous real-time media (such as voice or video calls).

A remote desktop session is interactive: every user action—typing, moving the mouse, scrolling, dragging windows—results in immediate network traffic and a visual response. The user therefore perceives even small delays more readily than during web browsing.

However, unlike voice or video, Remote Desktop protocols are designed to tolerate some variability through buffering, prediction, compression, and protocol optimizations.

---

# RTT Is the Most Important Metric

For virtually all Remote Desktop solutions (Microsoft RDP, Azure Virtual Desktop [4], Citrix HDX, VMware Horizon, etc.), **latency is the dominant factor** affecting user experience.

Every interaction involves a feedback loop:

```
Keyboard/Mouse
       ↓
   Network
       ↓
Remote Desktop
       ↓
Application
       ↓
Screen Update
       ↓
Network
       ↓
User Sees Result
```

Even though many optimizations reduce the number of round trips, this loop remains fundamentally latency-sensitive.

As RTT increases:

- typing begins to feel delayed,
- mouse movement loses precision,
- scrolling becomes less responsive,
- window dragging feels disconnected,
- users start "waiting" for the desktop.

The relationship is gradual rather than abrupt.

Approximate perception:

| RTT | Typical User Perception |
|------|-------------------------|
| <50 ms | Feels almost local |
| 50–100 ms | Very comfortable |
| 100–150 ms | Slight delay, still pleasant |
| 150–200 ms | Delay becomes noticeable |
| 200–300 ms | Clearly remote |
| >300 ms | Productivity begins to suffer |

This aligns closely with Microsoft's guidance for Azure Virtual Desktop [4] and with long-standing Citrix deployment experience.

---

# Packet Loss

Remote Desktop protocols are remarkably resilient to packet loss.

Unlike streaming media, they can retransmit lost information.

This means:

- the session usually survives,
- the screen eventually updates correctly,
- user data is rarely lost.

However, the user experience deteriorates quickly.

Packet loss causes:

- typing pauses,
- delayed screen refreshes,
- frozen images,
- cursor jumps,
- visible "catch-up" behavior.

Even though the protocol remains functional, users often describe the connection as:

> "laggy"

rather than

> "broken."

For this reason, the threshold where users begin noticing problems is considerably lower than the threshold where the protocol actually fails.

---

# Jitter

Jitter is more important than many administrators assume.

Historically, RDP relied almost exclusively on TCP, making RTT the overwhelming concern.

Modern Remote Desktop implementations increasingly use UDP for graphics, multimedia, and low-latency updates.

As a result, high jitter can produce:

- uneven mouse movement,
- inconsistent scrolling,
- jerky animations,
- fluctuating responsiveness,
- poor-quality video or Teams calls running inside the remote session.

Unlike voice calls, however, users generally tolerate more jitter because Remote Desktop protocols are less dependent on perfectly regular packet timing.

For typical office applications, RTT remains more important than jitter.

---

# Workload Matters

One of the strongest conclusions from the research is that there is **no single Remote Desktop threshold** suitable for every workload.

For example:

### Office Productivity

Applications such as:

- Word
- Excel
- ERP systems
- Email
- Accounting software

remain comfortable even around:

- **100–150 ms RTT**

because interactions are relatively discrete.

---

### Graphic Work

Applications involving:

- CAD,
- GIS,
- image editing,
- complex user interfaces,

are much more sensitive.

Frequent screen updates make both RTT and jitter more noticeable.

---

### Multimedia

Running:

- Microsoft Teams [1],
- Zoom [3],
- YouTube,
- training videos,

inside a Remote Desktop session introduces an entirely different workload.

At that point, the network characteristics begin to resemble those required for video conferencing rather than traditional desktop interaction.

---

# Why Microsoft and Citrix Numbers Seem So High

A recurring theme in vendor documentation is the distinction between:

- **acceptable**
- **comfortable**

For example, Citrix documentation often discusses operation at:

- **300 ms latency**
- **5% packet loss**

This does **not** mean Citrix recommends those values.

Instead, it means that Citrix has protocol optimizations that allow users to continue working under poor conditions.

Likewise, Microsoft documentation stating that Azure Virtual Desktop [4] can operate at relatively high latency should not be interpreted as meaning users will enjoy the experience.

These are survivability limits, not quality targets.

---

# Recommended Thresholds

Based on the combined evidence, the research recommends approximately:

| Classification | RTT | Packet Loss | Jitter |
|---------------|----:|------------:|--------:|
| **Good** | <80 ms | <0.5% | <20 ms |
| **Medium** | 80–150 ms | 0.5–2% | 20–40 ms |
| **Bad** | >250 ms | >5% | >80 ms |

These thresholds correspond much more closely to what users actually perceive than to the limits at which Remote Desktop software continues functioning.


---

# Part 6 — Calls and Gaming Analysis

Although audio/video calls and online gaming are often grouped together as "real-time applications," the research shows they respond differently to network impairments.

The common characteristic is that **latency, packet loss, and jitter all matter simultaneously**. Unlike web browsing, none of these metrics can be ignored.

However, the relative importance of each metric depends on the application.

---

# Audio Calls

Voice communication is arguably the most thoroughly researched Internet application.

Decades of ITU recommendations [2] and codec development have established a clear understanding of what users perceive as good quality.

The most important finding is:

> **Conversation latency matters more than audio fidelity.**

Humans naturally adapt to occasional audio imperfections.

They adapt much less successfully to conversational delay.

---

## RTT

Conversation begins to feel natural when RTT remains approximately below:

- **100–150 ms**

As RTT increases:

- participants begin speaking over one another,
- pauses become awkward,
- conversations require more conscious effort,
- discussions feel less fluid.

Above roughly **250–300 ms RTT**, normal conversation becomes noticeably more difficult, even if the audio itself remains clear.

---

## Packet Loss

Modern voice codecs are remarkably tolerant of packet loss.

They use techniques such as:

- packet-loss concealment,
- forward error correction,
- interpolation.

This means:

- **0.5–1% loss** is often inaudible,
- **1–3%** becomes noticeable,
- **above 3%** audio artifacts become increasingly frequent.

Users typically hear:

- clipped words,
- robotic speech,
- missing syllables,
- brief audio gaps.

---

## Jitter

Jitter directly affects real-time playback.

Packets arriving too early or too late must be absorbed by a **jitter buffer**.

Small jitter buffers reduce delay but increase the chance of dropped packets.

Large jitter buffers improve robustness but increase conversational latency.

This creates an unavoidable engineering trade-off.

Most vendors therefore recommend keeping jitter below approximately:

- **20–30 ms**

for consistently high-quality calls.

---

# Video Calls

Video conferencing follows almost the same principles as voice, but users tolerate visual degradation more readily than audio degradation.

When the network deteriorates, video systems usually adapt by:

- reducing resolution,
- lowering frame rate,
- decreasing bitrate.

Most users accept temporary visual degradation surprisingly well.

What they do **not** tolerate is delayed conversation.

Consequently:

- RTT remains the dominant metric.
- Packet loss is slightly less critical than for audio because modern video codecs conceal errors effectively.
- Jitter has similar importance to audio because irregular packet arrival affects both audio and video synchronization.

This explains why Microsoft Teams [1], Zoom [3], and Google Meet publish very similar latency and jitter recommendations despite using different codecs.

---

# Online Gaming

Gaming differs fundamentally from voice and video because **responsiveness is the primary objective**.

A video call can tolerate a modest delay as long as it remains consistent.

A competitive game cannot.

---

## RTT

RTT is the single most important metric for nearly all multiplayer games.

Latency directly affects:

- input responsiveness,
- hit registration,
- player movement,
- synchronization with the game server.

Approximate player perception:

| RTT | Typical Experience |
|-----:|--------------------|
| <20 ms | Excellent |
| 20–40 ms | Competitive |
| 40–60 ms | Very good |
| 60–100 ms | Noticeable delay |
| 100–150 ms | Clearly disadvantaged |
| >150 ms | Poor for competitive play |

These values align well with current expectations among players and developers.

---

## Packet Loss

Packet loss affects games differently depending on the networking model.

Many games use UDP.

Lost packets are simply lost.

Typical effects include:

- rubber-banding,
- teleporting,
- delayed hit registration,
- temporary freezes,
- desynchronization.

Modern games employ prediction algorithms to mask occasional loss, but they cannot compensate for sustained packet loss.

Even **1% loss** can be clearly noticeable in fast-paced games.

---

## Jitter

Gaming research consistently shows that **stable latency** is nearly as important as **low latency**.

Consider two Internet connections:

**Connection A**

- 30 ms RTT
- ±1 ms jitter

**Connection B**

- 30 ms RTT
- ±25 ms jitter

Average latency is identical.

Yet virtually every player would prefer Connection A.

High jitter causes:

- inconsistent movement,
- unpredictable aiming,
- varying hit registration,
- uneven game feel.

The player experiences the game as inconsistent rather than simply slow.

---

# Cloud Gaming

Cloud gaming is even more demanding than traditional online gaming.

The Internet connection now carries:

- controller input to the server,
- video stream back to the client.

Network latency therefore becomes only one component of total end-to-end latency.

The complete pipeline includes:

- controller latency,
- network latency,
- server processing,
- video encoding,
- network return path,
- video decoding,
- display latency.

For this reason, services such as NVIDIA GeForce NOW [11] and Xbox Cloud Gaming recommend significantly lower network latency than traditional multiplayer games.

---

# Superhuman Gaming

The "Superhuman Gaming" profile corresponds to what the research describes as an **elite-performance tier**.

Its purpose is not to determine whether a game is playable.

Instead, it asks:

> **"Is the network fast enough that it is unlikely to be the limiting factor for an elite player's performance?"**

Approximate thresholds:

| Metric | Target |
|---------|-------:|
| RTT | <15 ms |
| Packet loss | <0.1% |
| Jitter | <5 ms |

These values are intentionally aggressive.

Most home Internet connections will not consistently achieve them over long measurement periods.

---

# Relative Importance of the Metrics

The research suggests the following approximate importance for each profile:

| Profile | RTT | Packet Loss | Jitter |
|---------|----:|------------:|-------:|
| Audio Calls | **45%** | **25%** | **30%** |
| Video Calls | **45%** | **20%** | **35%** |
| Online Gaming | **50%** | **25%** | **25%** |
| Superhuman Gaming | **50%** | **20%** | **30%** |

These weightings are not based on a formal standard but synthesize guidance from standards bodies, vendor documentation, and empirical user experience.

The key takeaway is that **RTT remains the dominant factor across all real-time applications**, but **jitter becomes almost equally important for applications that require a continuous stream of packets**. Packet loss, while still important, is generally more tolerable because modern codecs and game engines include sophisticated mechanisms to compensate for occasional losses.

---

# Part 7 — Jitter Discussion and Measurement Methodology

One of the most valuable outcomes of this research is a clearer understanding of **what jitter actually measures** and **when it is useful**.

Many Internet quality tools display RTT, packet loss, and jitter together, which can create the impression that they are equally important for every application.

The evidence does not support this.

Jitter is **extremely important** for some workloads and **almost irrelevant** for others.

---

# What Is Jitter?

Latency (RTT) measures:

> **"How long does a packet take to travel?"**

Jitter measures:

> **"How much does that travel time vary from one packet to the next?"**

Example:

```
20
21
19
20
20
21
20
```

Average RTT:

20 ms

Jitter:

Very low.

---

Now compare:

```
20
60
15
45
18
70
22
```

Average RTT:

Approximately the same.

But the user experience is dramatically different.

The second connection feels:

- unpredictable,
- inconsistent,
- unstable.

This is precisely what jitter is intended to capture.

---

# Why Streaming Applications Care About Jitter

Audio and video are played back continuously.

Packets are expected to arrive at a regular pace.

If packets arrive irregularly:

```
Audio
██████████████████

Packet arrivals
███    ███████      ██
```

the receiver must decide whether to:

- wait for late packets,
- or continue playback.

Waiting increases delay.

Not waiting causes missing audio or video.

Every real-time application therefore maintains a **jitter buffer**.

The larger the jitter:

- the larger the required buffer,
- the greater the added latency.

This is why jitter has a direct effect on perceived call quality.

---

# Why Browsing Does Not Care

Browsing behaves completely differently.

A browser simply waits until enough data has arrived before rendering the page.

Whether packets arrive:

- evenly,
- in bursts,
- with small timing variations,

is largely irrelevant.

The user notices only:

- when the page starts loading,
- when it finishes.

This is why HTTP performance studies rarely mention jitter.

Instead, they focus on:

- RTT,
- throughput,
- page load time,
- transaction latency.

---

# Remote Desktop Is Somewhere in Between

Remote Desktop combines characteristics of both models.

Simple office tasks:

- typing,
- forms,
- spreadsheets,

are mostly influenced by RTT.

Continuous screen updates:

- scrolling,
- animations,
- video playback,

are more affected by jitter.

This explains why RDP becomes noticeably less smooth even when average RTT remains acceptable but packet timing becomes irregular.

---

# Gaming

Gaming is highly sensitive to jitter because modern games simulate a continuous world.

Players expect:

- movement,
- aiming,
- hit registration,

to behave consistently.

High jitter causes the game to alternate between:

- feeling responsive,
- feeling sluggish,

even if the average RTT remains unchanged.

Most players perceive this as:

> "The connection feels inconsistent."

rather than

> "The ping is high."

---

# Measuring Jitter Correctly

The research found substantial differences between implementations.

Some tools report:

- average jitter,

others:

- maximum jitter,

others:

- percentile jitter.

These can produce very different values.

For example:

```
RTT

20
20
20
20
20
80
20
20
20
20
```

Average jitter appears small.

Yet the user clearly notices the single 80 ms spike.

This is why modern monitoring increasingly relies on percentile statistics.

---

## RFC 3550 [9] (RTP)

The official RTP specification defines jitter using an exponentially smoothed estimate of packet-delay variation.

This works well for continuous media streams but is less suitable for periodic ICMP ping measurements.

---

# Measuring Jitter with Ping

For a network quality tool based on ICMP or UDP probes, a practical definition is:

> **Jitter = the average absolute difference between consecutive RTT measurements.**

Example:

```
20
21
19
24
22
```

Differences:

```
1
2
5
2
```

Average jitter:

2.5 ms

This method is:

- simple,
- intuitive,
- stable,
- widely implemented.

---

# Average vs Maximum vs Percentile

One of the strongest recommendations from the research concerns how jitter should be summarized.

### Average jitter

Advantages:

- Stable
- Easy to understand

Disadvantages:

- Can hide occasional spikes

---

### Maximum jitter

Advantages:

- Detects worst-case events

Disadvantages:

- Highly sensitive to a single outlier
- Poor indicator of typical user experience

---

### 95th or 99th Percentile

Advantages:

- Captures nearly all meaningful spikes
- Ignores rare anomalies
- Better matches user perception

Disadvantages:

- Slightly more complex to compute and explain

The research consistently favors **95th- or 99th-percentile latency and jitter** for connection quality assessment.

---

# Measurement Duration

The length of the measurement window significantly influences the results.

A very short test may miss intermittent problems.

A very long test can dilute transient issues.

Approximate guidance:

| Test Type | Typical Duration |
|-----------|-----------------:|
| Quick quality check | 20–30 seconds |
| Normal connection assessment | 60–120 seconds |
| Long-term monitoring | Continuous |

Long-term monitoring provides the most representative picture because it captures intermittent congestion, Wi-Fi interference, ISP routing changes, and other transient events that short tests often miss.

---

# Recommendations for Your Program

Based on the research, the following approach is recommended:

### RTT

- Measure continuously.
- Report:
  - average,
  - minimum,
  - **95th percentile**.

---

### Packet Loss

Measure over the entire test period.

Avoid making judgments based on only a handful of packets, as a single lost packet can disproportionately affect the calculated percentage.

---

### Jitter

Use it only for:

- Remote Desktop,
- Audio Calls,
- Video Calls,
- Gaming.

For Browsing & Email:

- omit it,
- or give it negligible weight.

---

### Bufferbloat

If feasible, add a fourth metric:

**Loaded Latency (Bufferbloat)**

This would substantially improve the classifier for:

- Browsing,
- Email,
- Cloud applications,
- general office productivity.

The research indicates that loaded latency is often a better predictor of user satisfaction than jitter for transactional Internet use.

---

# Part 8 — Classifier Design Recommendations

The threshold values themselves are only one part of a successful connection quality classifier. Equally important is **how those thresholds are combined** into an overall assessment.

The research strongly suggests that a classifier should not treat RTT, packet loss, and jitter as equally important or apply identical logic to every application profile.

Instead, the classification should reflect **how each application actually behaves**.

---

# Avoid a Simple Pass/Fail Model

One tempting approach is:

> If every metric is in the "Good" range → Good  
> If any metric is in the "Bad" range → Bad  
> Otherwise → Medium

While simple, this approach produces many results that users would consider incorrect.

For example:

| RTT | Loss | Jitter |
|----:|-----:|--------:|
| 18 ms | 0% | 35 ms |

A strict rule would classify this gaming connection as **Bad** because of jitter.

In reality, many players would still consider it very good.

Conversely:

| RTT | Loss | Jitter |
|----:|-----:|--------:|
| 140 ms | 0% | 2 ms |

All metrics except RTT appear excellent.

Yet almost every competitive gamer would complain about this connection.

This illustrates that **RTT is not interchangeable with jitter or packet loss**.

---

# Weighted Scoring Works Better

The research recommends assigning different weights to each metric depending on the usage profile.

Example:

| Profile | RTT | Loss | Jitter |
|---------|----:|------:|--------:|
| Browsing | 70% | 30% | 0% |
| Remote Desktop | 60% | 25% | 15% |
| Audio Calls | 45% | 25% | 30% |
| Video Calls | 45% | 20% | 35% |
| Gaming | 50% | 25% | 25% |

This produces classifications that align much more closely with user perception.

---

# Smooth Scoring Is Better Than Hard Thresholds

Another major recommendation is to avoid sudden jumps in quality.

Suppose your threshold is:

- Good RTT: **<40 ms**

A connection measured at:

- **39 ms** → Good

while:

- **40 ms** → Medium

There is no meaningful difference between these two connections, yet a threshold-based classifier produces a completely different result.

A smoother approach is to assign a score that gradually decreases as each metric worsens.

For example:

```
Excellent RTT → 100 points
Good RTT      → 80
Fair RTT      → 60
Poor RTT      → 40
Very Poor RTT → 20
```

Each metric contributes proportionally to the final score, avoiding abrupt changes due to tiny measurement differences.

---

# Profiles Should Not Share the Same Thresholds

A key conclusion from the research is that each profile genuinely requires different criteria.

For example:

| Metric | Browsing | Gaming |
|--------|----------:|--------:|
| RTT 120 ms | Good | Poor |
| Jitter 25 ms | Ignore | Medium |
| Loss 1% | Medium | Poor |

A single "Internet Quality Score" cannot accurately describe both experiences.

The same connection may be:

- Excellent for email,
- Acceptable for Teams,
- Unsuitable for esports.

This is a feature, not a flaw.

---

# Percentiles Are Better Than Averages

The research consistently recommends using **95th- or 99th-percentile values** instead of relying solely on averages.

Example:

```
RTT

20
20
20
20
20
150
20
20
20
20
```

Average RTT:

33 ms

Most users would describe this connection as having occasional lag spikes, not as a stable 33 ms connection.

The **95th percentile** captures those spikes much more effectively than the average.

This principle applies to:

- RTT,
- jitter,
- loaded latency.

---

# Packet Loss Should Be Measured Over Sufficient Time

Packet loss percentages become misleading when only a small number of packets are observed.

Example:

```
100 packets
1 lost

Loss = 1%
```

Versus:

```
10 packets
1 lost

Loss = 10%
```

The second result appears much worse despite representing only a single lost packet.

The recommendation is therefore to compute packet loss over a sufficiently large sample to ensure statistical stability.

---

# Consider Reporting Multiple Scores

Rather than presenting a single number, the research suggests that your software could report:

- **Browsing Quality**
- **Remote Desktop Quality**
- **Call Quality**
- **Gaming Quality**

Many professional monitoring platforms already take this approach because different applications stress different aspects of the network.

---

# Good vs Operational

Perhaps the most important design recommendation is to decide what the classifier is intended to answer.

There are two fundamentally different questions:

### 1. Does the application work?

This is how many vendors define quality.

Example:

- Teams meeting still functions.
- RDP session remains connected.
- Game is still playable.

These thresholds are relatively relaxed.

---

### 2. Does the application feel good?

This is how users judge quality.

The corresponding thresholds are significantly stricter.

Throughout the research, vendor dashboards were found to focus primarily on **operational health**, whereas engineering recommendations and user studies focused on **perceived quality**.

If your software is intended for end users or IT support staff evaluating Internet connections, the second interpretation is generally the more useful one.

---

# Overall Recommendation

The research suggests the following design principles:

1. **Use different thresholds for each application profile.**
2. **Weight RTT more heavily than the other metrics.**
3. **Do not use jitter for Browsing & Email.**
4. **Use smoother scoring rather than abrupt threshold changes where possible.**
5. **Prefer 95th-percentile metrics over simple averages.**
6. **Measure packet loss over enough packets to produce stable percentages.**
7. **If practical, add loaded latency (bufferbloat) [1][12] as a fourth metric**, especially for browsing and general office productivity.

Taken together, these principles produce a classifier that aligns closely with both modern networking research and how users actually perceive Internet quality, rather than simply reflecting whether applications continue to function.

---

# Part 9 — References and Implementation Notes

This section summarizes the principal sources used in the research and distills them into practical implementation guidance.

---

# Primary Standards

## ITU-T G.1010

One of the most influential recommendations for application-oriented network quality.

It classifies applications such as:

- voice,
- video,
- web browsing,
- interactive gaming,

and provides target performance levels for:

- latency,
- packet loss,
- response time.

One of its most important observations is that **web browsing is evaluated primarily by response time**, not by jitter.

---

## ITU-T G.114

The classic recommendation for conversational voice latency.

Key conclusions:

- Below approximately **150 ms one-way delay**, conversation feels natural.
- Between **150 and 400 ms**, communication gradually becomes more difficult.
- Above **400 ms**, interactive conversation becomes increasingly uncomfortable.

Although written many years ago, these thresholds remain broadly applicable.

---

## ITU-T Y.1541

Defines IP Quality-of-Service (QoS) classes.

Provides engineering targets for:

- delay,
- packet-delay variation (jitter),
- packet loss.

These values are intended for **network design**, not for consumer Internet quality scoring.

---

# Vendor Documentation

The research compared recommendations from several major vendors.

## Microsoft

Sources included:

- Microsoft Teams [1] network requirements,
- Azure Virtual Desktop [4] guidance,
- Microsoft Teams [1] call quality metrics.

One important finding is that Microsoft publishes **two different categories of thresholds**:

1. **Planning thresholds**, used to design high-quality networks.
2. **Operational thresholds**, used to determine whether services are still functioning acceptably.

Understanding this distinction is essential when designing a user-facing quality classifier.

---

## Google

Documentation from:

- Google Meet,
- Google Voice [7],

provided recommendations for:

- latency,
- jitter,
- packet loss,

along with explanations of how these metrics affect media quality.

---

## Zoom [3]

Zoom [3]'s published network requirements closely align with Microsoft's planning guidance.

They provide practical thresholds for maintaining high-quality audio and video calls.

---

## Citrix

Citrix documentation was particularly valuable for understanding Remote Desktop behavior under adverse network conditions.

It clearly distinguishes between:

- protocol survivability,
- user experience.

---

## NVIDIA and Xbox

Cloud gaming documentation highlighted the increasing importance of:

- low latency,
- stable latency,
- minimal packet loss,

for interactive streaming applications.

---

# Academic Literature

The research also reviewed academic work on:

- multiplayer gaming latency,
- user perception,
- human response times,
- Internet performance measurement.

Several consistent themes emerged:

- Users detect inconsistent latency more readily than average latency.
- Stable latency is often more important than the absolute minimum RTT.
- Small amounts of packet loss have a larger impact on interactive applications than many older threshold tables suggest.

---

# Cloudflare [12]

Cloudflare [12]'s Internet Quality Score proved particularly influential because it emphasizes **loaded latency (bufferbloat) [1][12]** rather than relying solely on idle latency measurements.

This reflects a growing industry trend toward evaluating networks under realistic load conditions instead of only when they are idle.

---

# Key Implementation Recommendations

The research suggests implementing the classifier using the following principles.

---

## RTT

Compute and report:

- Minimum RTT
- Average RTT
- **95th-percentile RTT**

The percentile value is especially useful because it captures intermittent latency spikes that users often perceive as lag.

---

## Packet Loss

Measure packet loss over a sufficiently large sample to produce stable percentages.

Avoid drawing conclusions from only a few packets.

---

## Jitter

Use jitter for:

- Remote Desktop,
- Audio Calls,
- Video Calls,
- Gaming.

Do **not** use it as a primary metric for:

- Browsing,
- Email.

---

## Bufferbloat

If possible, include:

- **Idle RTT**
- **Loaded RTT**

The difference between the two provides a direct measure of bufferbloat.

This metric often predicts browsing and office application responsiveness better than jitter.

---

## Measurement Duration

Recommended durations:

| Purpose | Duration |
|---------|---------:|
| Quick assessment | 20–30 seconds |
| Standard quality assessment | 60–120 seconds |
| Long-term monitoring | Continuous |

Longer measurements better capture intermittent congestion and transient network problems.

---

## Multiple Profiles

Rather than reporting a single overall quality score, the research recommends evaluating the connection separately for each intended use case.

For example, a single connection might legitimately be classified as:

| Usage Profile | Result |
|---------------|--------|
| Browsing & Email | Excellent |
| Remote Desktop | Good |
| Audio Calls | Good |
| Video Calls | Medium |
| Online Gaming | Poor |
| Superhuman Gaming | Unsuitable |

This reflects reality far better than forcing every workload into one generic "Internet Quality" rating.


---

# References

The following references correspond to the primary sources cited throughout the original Deep Research report.

[1] Microsoft. *Microsoft 365 Network Connectivity Test Tool*. Microsoft Learn.  
https://learn.microsoft.com/en-us/microsoft-365/enterprise/office-365-network-mac-perf-onboarding-tool

[2] ITU-T. *Recommendation G.1010 – End-user multimedia QoS categories*.  
https://www.itu.int/rec/T-REC-G.1010

[3] Zoom [3]. *Accessing Meeting and Phone Statistics*.  
https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0070504

[4] Microsoft. *Troubleshoot Azure Virtual Desktop [4] Connection Quality*. Microsoft Learn.  
https://learn.microsoft.com/en-us/troubleshoot/azure/virtual-desktop/troubleshoot-connection-quality

[5] Electronic Arts. *What is Connection Quality in EA [5] SPORTS FC™?*  
https://help.ea.com/en/articles/ea-sports-fc/connection-quality/

[6] NVIDIA Research [6]. *Latency of 30 ms Benefits First Person Targeting Tasks More Than Refresh Rate Above 60 Hz*.  
https://research.nvidia.com/publication/2019-11_latency-30-ms-benefits-first-person-targeting-tasks-more-refresh-rate-above-60

[7] Google Workspace. *Troubleshoot Google Voice [7] Call Quality*.  
https://knowledge.workspace.google.com/admin/voice/troubleshoot-google-voice-call-quality

[8] Microsoft. *Optimization of RDP Traffic*. Microsoft Learn.  
https://learn.microsoft.com/en-us/windows-365/enterprise/optimization-of-rdp

[9] IETF. *RFC 3550 [9] – RTP: A Transport Protocol for Real-Time Applications*.  
https://datatracker.ietf.org/doc/html/rfc3550

[10] IETF. *RFC 5481 [10] – Packet Delay Variation Applicability Statement*.  
https://www.rfc-editor.org/rfc/rfc5481.html

[11] NVIDIA. *How do I test my network for GeForce NOW [11]?*  
https://nvidia.custhelp.com/app/answers/detail/a_id/5224/

[12] Cloudflare [12]. *Aggregated Internet Measurement (Internet Quality Score)*.  
https://developers.cloudflare.com/speed/aim/

## Further Reading

These references were discussed in the report but were not listed in the exported bibliography:

- ITU-T Recommendation G.114 – One-way transmission time.
- ITU-T Recommendation Y.1541 – Network performance objectives for IP-based services.
- Microsoft Teams [1] networking guidance and Call Quality Dashboard documentation.
- Google Meet network recommendations.
- Microsoft Windows 365 [8] / Azure Virtual Desktop [4] networking guidance.
