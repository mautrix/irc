# Features & roadmap
* Matrix → IRC
  * [ ] Message content
    * [x] Plain text
    * [ ] Formatted messages
    * [x] Media/files (as links)
  * [x] Replies with [`+draft/reply`](https://ircv3.net/specs/client-tags/reply.html)
  * [x] Reactions with [`+draft/react`](https://ircv3.net/specs/client-tags/reaction.html)
  * [x] Redactions with [`draft/message-redaction`](https://ircv3.net/specs/extensions/message-redaction)
  * [ ] Optional plaintext fallback for replies
  * [ ] Room metadata changes
    * [ ] Name
    * [ ] Topic
* IRC → Matrix
  * [ ] Message content
    * [x] Plain text
    * [ ] Formatted messages
  * [x] Replies with [`+draft/reply`](https://ircv3.net/specs/client-tags/reply.html)
  * [x] Reactions with [`+draft/react`](https://ircv3.net/specs/client-tags/reaction.html)
  * [x] Redactions with [`draft/message-redaction`](https://ircv3.net/specs/extensions/message-redaction)
  * [ ] Channel metadata changes
  * [x] Initial channel metadata
  * [x] User nick changes
* Misc
  * [ ] Private chat creation by inviting Matrix ghost of IRC user to new room
