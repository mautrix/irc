# Features & roadmap
* Matrix → IRC
  * [ ] Message content
    * [x] Plain text
    * [ ] Formatted messages
    * [x] Media/files (as links)
  * [x] Replies ([`+draft/reply`](https://ircv3.net/specs/client-tags/reply.html))
  * [ ] Optional plaintext fallback for replies
  * [x] Reactions ([`+draft/react`](https://ircv3.net/specs/client-tags/reaction.html))
  * [x] Redactions ([`draft/message-redaction`](https://ircv3.net/specs/extensions/message-redaction))
  * [x] Typing notifications ([`typing`](https://ircv3.net/specs/client-tags/typing))
  * [ ] Room metadata changes
    * [ ] Name
    * [ ] Topic
* IRC → Matrix
  * [ ] Message content
    * [x] Plain text
    * [ ] Formatted messages
  * [x] Replies ([`+draft/reply`](https://ircv3.net/specs/client-tags/reply.html))
  * [x] Reactions ([`+draft/react`](https://ircv3.net/specs/client-tags/reaction.html))
  * [x] Redactions ([`draft/message-redaction`](https://ircv3.net/specs/extensions/message-redaction))
  * [x] Typing notifications ([`typing`](https://ircv3.net/specs/client-tags/typing))
  * [ ] Channel metadata changes
  * [x] Initial channel metadata
  * [x] User nick changes
* Misc
  * [ ] Private chat creation by inviting Matrix ghost of IRC user to new room
