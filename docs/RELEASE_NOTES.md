# DigitDNS v5.2.13 — agent control plane without the restart stampede

**Restarting the hub no longer makes every agent pull a full catalogue.** The
control-plane revision is now persisted, and the catalogue ETag carries a build
fingerprint:

```
ETag: "v<N>-<buildhash>"
```

- Same build, unchanged config, hub restarted → every parked agent gets **304**,
  no rebuild.
- A real upgrade changes the build hash, so agents refresh exactly once.
- A config change still bumps the counter and wakes the parked long-polls
  immediately.

**A config change is now O(1) in database work.** The rules and panel client list
behind every catalogue are cached per revision; only the per-agent assignment
walk runs. Before this, one change woke N agents and each re-read both inputs.

**Heartbeats are cheaper.** They share a batched bbolt transaction (one fsync for
beats arriving together) and the write returns the updated agent, so the handler
no longer performs a second read to update the live relay router.

**The safety sync is rarer, and reconnects are faster.** The agent's forced full
fetch moved from every 5 minutes to every 30; normal propagation is still the
long-poll. A transport-level drop (hub restart or an idle proxy) is no longer
treated like an HTTP rejection: the next successful heartbeat re-arms the poll
immediately, while genuine errors and 5xx keep the exponential backoff.

**New metrics** for the control plane: catalogue requests, 304s, builds, parked
long-polls, revision, agents online and agents total.

No resolver, relay or storage-format changes. The only new setting record is
`agent_revision`.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.2.13
docker pull ghcr.io/digitrob/digitdns-agent:5.2.13
docker pull robmeister/digitdns:5.2.13
docker pull robmeister/digitdns-agent:5.2.13
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.2.13/scripts/install.sh | sudo bash
```

Agents may be updated independently; older agents keep working, but only 5.2.13
agents get the batched heartbeat, persisted-revision 304s, the 30-minute safety
sync and the fast re-arm.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.13-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.13` tag.
