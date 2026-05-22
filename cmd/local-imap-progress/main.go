package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"local-imap-mcp/internal/config"
	"local-imap-mcp/internal/imapclient"
)

type snapshot struct {
	At     time.Time
	Count  *imapclient.MailboxCount
	Latest *imapclient.MessageSummary
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	mailbox := flag.String("mailbox", "", "mailbox to inspect, default is config imap.default_mailbox")
	watch := flag.Bool("watch", false, "repeat until interrupted")
	interval := flag.Duration("interval", 10*time.Second, "watch interval")
	target := flag.Int("target", 0, "optional remote target message count for percent and ETA")
	flag.Parse()

	log.SetOutput(io.Discard)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}
	if *mailbox == "" {
		*mailbox = cfg.IMAP.DefaultMailbox
	}

	client := imapclient.New(cfg)
	var previous *snapshot
	for {
		current, err := collect(client, *mailbox)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		printSnapshot(current, previous, *target)

		if !*watch {
			return
		}
		previous = current
		time.Sleep(*interval)
	}
}

func collect(client *imapclient.Client, mailbox string) (*snapshot, error) {
	count, err := client.CountMessages(mailbox)
	if err != nil {
		return nil, err
	}

	var latest *imapclient.MessageSummary
	headers, err := client.SampleRecentHeaders(mailbox, 1)
	if err != nil {
		return nil, err
	}
	if len(headers) > 0 {
		latest = &headers[0]
	}

	return &snapshot{
		At:     time.Now(),
		Count:  count,
		Latest: latest,
	}, nil
}

func printSnapshot(current, previous *snapshot, target int) {
	fmt.Println("local-imap-mcp import progress")
	fmt.Println(strings.Repeat("=", 31))
	fmt.Printf("time:      %s\n", current.At.Format(time.RFC3339))
	fmt.Printf("mailbox:   %s\n", current.Count.Mailbox)
	fmt.Printf("messages:  %d\n", current.Count.Messages)
	fmt.Printf("uidNext:   %d\n", current.Count.UIDNext)
	fmt.Printf("uidValid:  %d\n", current.Count.UIDValidity)
	fmt.Printf("recent:    %d\n", current.Count.Recent)

	if previous != nil {
		elapsed := current.At.Sub(previous.At)
		delta := int64(current.Count.Messages) - int64(previous.Count.Messages)
		ratePerMinute := 0.0
		if elapsed > 0 {
			ratePerMinute = float64(delta) / elapsed.Minutes()
		}
		fmt.Printf("delta:     %+d in %s (%.1f msg/min)\n", delta, elapsed.Round(time.Second), ratePerMinute)
		printTarget(current, target, ratePerMinute)
	} else {
		printTarget(current, target, 0)
	}

	if current.Latest != nil {
		fmt.Println()
		fmt.Println("latest exposed message")
		fmt.Println("----------------------")
		fmt.Printf("seq/uid:   %d / %d\n", current.Latest.SeqNum, current.Latest.UID)
		fmt.Printf("date:      %s\n", emptyDash(current.Latest.Date))
		fmt.Printf("internal:  %s\n", emptyDash(current.Latest.InternalDate))
		fmt.Printf("from:      %s\n", emptyDash(strings.Join(current.Latest.From, ", ")))
		fmt.Printf("subject:   %s\n", emptyDash(current.Latest.Subject))
	} else {
		fmt.Println()
		fmt.Println("latest exposed message: none")
	}
	fmt.Println()
}

func printTarget(current *snapshot, target int, ratePerMinute float64) {
	if target <= 0 {
		fmt.Println("target:    unknown (pass -target REMOTE_MESSAGES for percent/ETA)")
		return
	}

	remaining := target - int(current.Count.Messages)
	if remaining < 0 {
		remaining = 0
	}
	percent := 0.0
	if target > 0 {
		percent = float64(current.Count.Messages) * 100 / float64(target)
	}
	fmt.Printf("target:    %d (%.2f%%, %d remaining)\n", target, percent, remaining)

	if remaining == 0 {
		fmt.Println("eta:       complete or local count is above target")
		return
	}
	if ratePerMinute <= 0 {
		fmt.Println("eta:       waiting for a positive rate sample")
		return
	}
	eta := time.Duration(float64(remaining) / ratePerMinute * float64(time.Minute)).Round(time.Second)
	fmt.Printf("eta:       %s at current rate\n", eta)
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
