# Features & roadmap
* Matrix → IRC
  * [ ] Message content
    * [x] Plain text
    * [x] Formatted messages
    * [x] Media/files (as links)
    * [ ] Multiline messages ([`draft/multiline-messages`](https://ircv3.net/specs/extensions/multiline))
    * [ ] Splitting fallback for multiline messages
  * [x] Replies ([`+draft/reply`](https://ircv3.net/specs/client-tags/reply.html))
  * [ ] Optional plaintext fallback for replies
  * [x] Reactions ([`+draft/react`](https://ircv3.net/specs/client-tags/reaction.html))
  * [x] Redactions ([`draft/message-redaction`](https://ircv3.net/specs/extensions/message-redaction))
  * [x] Typing notifications ([`typing`](https://ircv3.net/specs/client-tags/typing))
  * [ ] Room metadata changes
    * [ ] Name
    * [x] Topic
* IRC → Matrix
  * [ ] Message content
    * [x] Plain text
    * [x] Formatted messages
    * [x] Multiline messages ([`draft/multiline-messages`](https://ircv3.net/specs/extensions/multiline))
  * [x] Server time ([`server-time`](https://ircv3.net/specs/extensions/server-time))
  * [x] Message IDs ([`message-ids`](https://ircv3.net/specs/extensions/message-ids))
  * [x] Replies ([`+draft/reply`](https://ircv3.net/specs/client-tags/reply.html))
  * [x] Reactions ([`+draft/react`](https://ircv3.net/specs/client-tags/reaction.html))
  * [x] Redactions ([`draft/message-redaction`](https://ircv3.net/specs/extensions/message-redaction))
  * [x] Typing notifications ([`typing`](https://ircv3.net/specs/client-tags/typing))
  * [ ] Backfilling messages ([`chathistory`](https://ircv3.net/specs/batches/chathistory))
  * [ ] Channel metadata changes
    * [ ] Name
    * [x] Topic
  * [x] Initial channel metadata
  * [x] User nick changes
* Misc
  * [ ] Private chat creation by inviting Matrix ghost of IRC user to new room
