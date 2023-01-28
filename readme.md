# Acme-Notmuch

A fork of [farhaven/acme-notmuch](https://github.com/farhaven/acme-notmuch).

## Changes

- Adds the ability to compose a new message to an address by passing the address with the -addr flag
- Prev/Next/NextUnread commands to page emails in a thread in-place. The message being viewed is replaced by the previous/next/next unread message in a thread
- Reply/ReplyAll commands for replying
- Tag messages as "replied" once a reply has been sent
- Removed a lot of the debug logging and pointless error paths (such as an error window for failing to render certain MIME types)

### Plumb rule

These plumb rules handle email addresses in Notmuch. There is one for mailto: links, and another for raw addresses.

```
type is text
data matches 'mailto:([a-zA-Z0-9_+.\-]+@[a-zA-Z0-9_+.\-]*)'
plumb start acme-notmuch -addr $1

type is text
data matches '[a-zA-Z0-9_+.\-]+@[a-zA-Z0-9_+.\-]*'
plumb start acme-notmuch -addr $0
```

---

# Original Readme

# Acme-Notmuch

This is a WIP mail reader for Acme, using Notmuch as the mail storage and query engine.

There are a few things missing that are required to make this useful:

* [x] Removing the `unread` tag from read messages
* [x] Mail authoring
	* [x Reply to some mail
	* [x] Write an initial mail
* [ ] Listing and saving attachments
* [ ] Spam handling with bogofilter
	* [ ] Mark messages as Ham/Spam
* [ ] Switch between `text/plain` or `text/html` view for `multipart/alternative` messages
	* Currently, if a `text/html` part exists, it is rendered as text and shown.
	* If there is none, whatever the first part is will be shown

The following things _do_ work:

* Running queries and showing the results
* Showing messages, including rough HTML -> Text conversion for messages with MIME content type "text/html"
* Jumping to the next unread message in the thread of the currently open message

## Requirements
* Acme
* Mail stored in a Notmuch database
* The `notmuch` command somewhere in your path