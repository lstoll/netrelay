# tunnel

A grab bag of things to tunnel traffic over http/X

Mostly a vibe hack project for fun. The goal is to be able to run a remote proxy on a tailscale funnel endpoint, and use a local endpoint to send traffic to it using OIDC auth. This is not the smartest or best way, but it scratched an itch.

Better ways:
* use tailscale
* use the tailscale CLI SOCKS5 proxy

Also a starting point to move to a full QUIC/MASQUE proxy, hopefully supporting Apples's Network Relay.
