package main

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"sync"

	"9fans.net/go/acme"
)

/* TODO:
- Add attachments to messages
	- Take a page from Mail's book: references to files end up as included, e.g. !attach /path/to/file at the beginning of a line?
	- Make "Attachments" a separate window?
	- Just a list of files to attach?
- Read ~/.signature
- Allow specifying the mail template somehow
- Sanity check mail:
	- check for attachments on things like "i have attached..."
*/

const newMailTemplate = `From:
To:
Subject:

`

const composeTemplate = `From:
To: %s
Subject:

`

func sendMessage(win *acme.Win) error {
	body, err := win.ReadAll("body")
	if err != nil {
		return err
	}

	cmd := exec.Command("msmtp", "--read-recipients", "--read-envelope-from")
	cmd.Stdin = bytes.NewBuffer(body)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("can't run msmtp: %q: %w", output, err)
	}

	if len(output) != 0 {
		win.Errf("got output from msmtp: %q", output)
	}

	return nil
}

func composeReply(wg *sync.WaitGroup, win *acme.Win, messageID string, replyToArg string) error {
	// win.Errf("composing reply for %s", messageID)

	cmd := exec.Command("notmuch", "reply", replyToArg, "id:"+messageID)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("notmuch-reply: %w", err)
	}

	wg.Add(1)
	go composeMessage(wg, string(output), messageID)

	return nil
}

func markReplied(messageID string) error {
	cmd := exec.Command("notmuch", "tag", "+replied", "id:"+messageID)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("notmuch-tag: %w", err)
	}
	if len(output) > 0 {
		return fmt.Errorf("output from notmuch: %q", output)
	}

	return nil
}

// FIXME: 'inReplyTo' is pretty hacky
func composeMessage(wg *sync.WaitGroup, initialText string, inReplyTo string) {
	defer wg.Done()

	win, err := newWin("/Mail/newMessage", "TODO")
	if err != nil {
		log.Printf("can't create window: %s", err)
		return
	}

	err = win.Fprintf("tag", " Send |fmt ")
	if err != nil {
		win.Errf("can't update tag: %s", err)
		return
	}

	err = win.Fprintf("body", "%s", initialText)
	if err != nil {
		win.Errf("can't write mail template to body: %s", err)
		return
	}

	for evt := range win.EventChan() {
		switch evt.C2 {
		case 'l', 'L':
			err := win.WriteEvent(evt)
			if err != nil {
				win.Errf("can't write window event: %s", err)
				return
			}
		case 'x', 'X':
			switch string(evt.Text) {
			case "Send":
				err := sendMessage(win)
				if err != nil {
					win.Errf("Can't send message: %s", err)
					continue
				}
				err = markReplied(inReplyTo)
				if err != nil {
					win.Errf("Can't mark replied: %s", err)
				}
				win.Del(true)
			default:
				err := win.WriteEvent(evt)
				if err != nil {
					win.Errf("can't write window event: %s", err)
					return
				}
			}
		default:
			continue
		}
	}
}
