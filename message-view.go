package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/mail"
	"os/exec"
	"strings"
	"sync"
	"time"

	"9fans.net/go/acme"
	"github.com/pkg/errors"

	"github.com/VictoriaLacroix/acme-notmuch/message"
)

// Set to false to disable removal of "unread" tag on message open
const _removeUnreadTag = true

func tagMessage(tags string, messageID string) error {
	args := []string{
		"tag",
	}

	args = append(args, strings.Fields(tags)...)

	args = append(args, []string{
		"--",
		"id:" + messageID,
	}...)

	cmd := exec.Command("notmuch", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("can't set tags: %q, %w", output, err)
	}

	return nil
}

func getThread(id string) (Thread, error) {
	// TODO: Handle multiple threads?

	cmd := exec.Command("notmuch", "search", "--format=json", "--output=threads", "id:"+ id)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var threadIDs []string
	err = json.Unmarshal(output, &threadIDs)
	if err != nil {
		return nil, err
	}

	if len(threadIDs) == 0 {
		return nil, errors.New("can't find thread for message")
	} else if len(threadIDs) > 1 {
		return nil, errors.New("more than one thread id for message")
	}

	// Get thread for the message
	cmd = exec.Command("notmuch", "show", "--format=json", "--body=false", "thread:"+threadIDs[0])
	output, err = cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var thread Thread
	err = json.Unmarshal(output, &thread)
	if err != nil {
		return nil, err
	}

	return thread, nil
}

func traverseTagSet(win *acme.Win, id *string, l []TagSet, unreadOnly bool) error {
	foundThisMsg := false
	foundNextMsg := false
	for _, entry := range l {
		if entry.MsgID == *id {
			foundThisMsg = true
			continue
		}

		if !foundThisMsg {
			continue
		}

		if !unreadOnly || entry.Tags["unread"] {
			// FIXME: Maybe make this less hacky
			*id = entry.MsgID
			refreshMessage(*id, win)
			err := tagMessage("-unread", *id)
			if err != nil {
				win.Errf("can't remove 'unread' tag from message %s", id)
			}
			foundNextMsg = true
			break
		}
	}

	if !foundThisMsg {
		return errors.New("current message not found in thread")
	}

	if !foundNextMsg {
		return errors.New("no next message found")
	}

	return nil
}

// prevMessage returns the message ID of the previous message in the same thread as id
func prevMessage(win *acme.Win, id *string, unreadOnly bool) error {
	thread, err := getThread(*id)

	if err != nil {
		return err
	}
	if thread == nil {
		return errors.New("No thread")
	}

	l := thread.PostOrder()

	return traverseTagSet(win, id, l, unreadOnly)
}

// nextMessage returns the message ID of the next message in the same thread as id
func nextMessage(win *acme.Win, id *string, unreadOnly bool) error {
	thread, err := getThread(*id)

	if err != nil {
		return err
	}
	if thread == nil {
		return errors.New("No thread")
	}

	l := thread.PreOrder()

	return traverseTagSet(win, id, l, unreadOnly)
}

func getAllHeaders(root message.Root) (mail.Header, error) {
	cmd := exec.Command("notmuch", "show", "--format=raw", "id:"+root.ID)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	msg, err := mail.ReadMessage(bytes.NewBuffer(output))
	if err != nil {
		return nil, err
	}

	// Add tags as a pseudo-header
	msg.Header["Tags"] = []string{strings.Join(root.Tags, ", ")}
	msg.Header["Crypto"] = []string{fmt.Sprintf("%v", root.Crypto)}

	return msg.Header, nil
}

func writeMessageHeaders(win *acme.Win, msg message.Root) error {
	allHeaders, err := getAllHeaders(msg)
	if err != nil {
		return errors.Wrap(err, "getting headers")
	}

	var errs []error

	date, err := allHeaders.Date()
	if err != nil {
		errs = append(errs, fmt.Errorf("can't read date: %w", err))
		date = time.Unix(0, 0)
	}

	headers := []string{"Date:\t" + date.Format(time.RFC3339)}

	addrHeaders := []string{"from", "to", "cc", "bcc"}
	for _, hdr := range addrHeaders {
		addrs, err := allHeaders.AddressList(hdr)
		if err != nil {
			if err == mail.ErrHeaderNotPresent {
				continue
			}

			return errors.Wrap(err, "reading address header")
		}

		var vals []string

		for _, addr := range addrs {
			vals = append(vals, addr.String())
		}

		headers = append(headers, strings.Title(hdr)+":\t"+strings.Join(vals, ", "))
	}

	moreHeaders := []string{"reply-to", "list-id", "x-bogosity", "content-type", "subject", "tags"}
	for _, hdr := range moreHeaders {
		val := allHeaders.Get(hdr)

		if val == "" {
			continue
		}

		headers = append(headers, strings.Title(hdr)+":\t"+val)
	}

	crypto := msg.Crypto.Render("\t")
	if crypto != "" {
		headers = append(headers, "Crypto:"+crypto)
	}

	if len(errs) != 0 {
		err = win.Fprintf("body", "Errors during processing:\n")
		if err != nil {
			return errors.Wrap(err, "writing to window")
		}
		for _, err := range errs {
			err = win.Fprintf("body", "%s\n", err.Error())
			if err != nil {
				return errors.Wrap(err, "writing to window")
			}
		}
	}

	win.PrintTabbed(strings.Join(headers, "\n"))

	return nil
}

func refreshMessage(messageID string, win *acme.Win) error {
	// TODO: Decode PGP
	cmd := exec.Command("notmuch", "show", "--decrypt=true", "--format=json", "--entire-thread=false", "--include-html", "id:"+messageID)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("loading payload: %w", err)
	}

	var msg message.Root
	err = json.Unmarshal(output, &msg)
	if err != nil {
		return fmt.Errorf("decoding message: raw=%s %w", output, err)
	}

	win.Clear()

	err = win.Name("/Mail/message/"+messageID)
	if err != nil {
		return fmt.Errorf("rewriting name for win %q: %w", messageID, err)
	}

	err = writeMessageHeaders(win, msg)
	if err != nil {
		return fmt.Errorf("writing headers for %q: %w", messageID, err)
	}

	err = win.Fprintf("body", "\n%s", msg.Render())
	if err != nil {
		return fmt.Errorf("writing message body: %w", err)
	}

	err = winClean(win)
	if err != nil {
		return fmt.Errorf("cleaning window state: %w", err)
	}

	return nil
}

func displayMessage(wg *sync.WaitGroup, messageID string) {
	// TODO:
	// - "Attachments" command
	//   - opens a new window with the attachments (MIME parts) listed, allows saving them somewhere
	//   - Decode base64
	// - Add "Headers" command to show full list of headers

	defer wg.Done()

	win, err := newWin("/Mail/message/"+messageID, "\nPrev Next NextUnread Reply ReplyAll Tag +flagged  |fmt ")
	if err != nil {
		win.Errf("can't open message display window for %s: %s", messageID, err)
		return
	}

	err = win.Fprintf("data", "Looking for message %s", messageID)
	if err != nil {
		win.Errf("can't write to body: %s", err)
		return
	}

	err = refreshMessage(messageID, win)
	if err != nil {
		win.Errf("can't refresh message: %s", err)
		return
	}

	if _removeUnreadTag {
		err = tagMessage("-unread", messageID)
		if err != nil {
			win.Errf("can't remove 'unread' tag from message %s", messageID)
			return
		}
	}

	for evt := range win.EventChan() {
		// Only listen to l and L events to catch right click on a thread ID
		// x and X go right back to acme
		switch evt.C2 {
		case 'x', 'X':
			cmd, arg := getCommandArgs(evt)

			switch cmd {
			case "Prev":
				err := prevMessage(win, &messageID, false)
				if err != nil {
					win.Errf("can't jump to prev message: %s", err)
				}
				continue
			case "Next":
				err := nextMessage(win, &messageID, false)
				if err != nil {
					win.Errf("can't jump to next message: %s", err)
				}
				continue
			case "NextUnread":
				err := nextMessage(win, &messageID, true)
				if err != nil {
					win.Errf("can't jump to next unread message: %s", err)
				}
				continue
			case "Reply":
				err := composeReply(wg, win, messageID, "--reply-to=sender")
				if err != nil {
					win.Errf("can't compose reply: %s", err)
				}
				continue
			case "ReplyAll":
				err := composeReply(wg, win, messageID, "--reply-to=all")
				if err != nil {
					win.Errf("can't compose reply all: %s", err)
				}
			case "Tag":
				err := tagMessage(arg, messageID)
				if err != nil {
					win.Errf("can't update tags: %s", err)
				}

				err = refreshMessage(messageID, win)
				if err != nil {
					win.Errf("can't refresh message: %s", err)
					return
				}

				continue
			}

			err := handleCommand(wg, win, evt)
			switch err {
			case nil:
				// Nothing to do, event already handled
			case errNotACommand:
				// Let ACME handle the event
				err := win.WriteEvent(evt)
				if err != nil {
					return
				}
			default:
				win.Errf("can't handle event: %s", err)
			}

			continue
		case 'l', 'L':
			err := win.WriteEvent(evt)
			if err != nil {
				win.Errf("can't write event: %s", err)
				return
			}

		default:
			continue
		}
	}
}
