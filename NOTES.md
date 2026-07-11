# Measurement notes

## Internet targets

The default Internet group uses a small set of well-known public targets. A
batch succeeds when at least one target replies on time, and Pingaro uses the
lowest RTT from the on-time replies.

This reduces false alarms caused by one public host having a temporary issue.
Packet loss for a group means no configured target in that group produced an
on-time reply for that scheduled sample.

## Scheduling

Pingaro schedules measurement batches at fixed intended times. A slow or lost
request does not delay later samples.

Each request has its own reply deadline. Replies received after the deadline are
late and cannot change the already finalized sample. Locally skipped requests,
such as requests skipped because an outstanding-request limit was reached, are
kept distinct from network packet loss.

If the process is suspended or delayed, Pingaro skips missed schedule slots
instead of sending a burst of overdue measurements.

## Historical note

Older versions used `ping.exe` subprocesses, which made loss timing dependent on
process behavior and localized command output. Current monitoring no longer uses
that path.
