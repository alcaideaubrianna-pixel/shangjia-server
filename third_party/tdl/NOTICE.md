# tdl attribution

This project adapts the following ideas and implementation details from
[`iyear/tdl`](https://github.com/iyear/tdl):

- lazy per-DC Telegram transfer-client pooling;
- independent lifecycle and invalidation for a DC transfer pool;
- 1 MiB parallel download parts and size-based download thread selection.

The adapted code is maintained in
`server/addons/youban_publish/logic/sys/tg_media_transfer.go` and is adjusted
for this project's gotd version and runtime. The original project is licensed
under AGPL-3.0. Commercial authorization from the tdl author is being obtained
separately; retain the authorization record with the project's legal notices.
