package main

import (
	"fmt"
	"os"

	logger "github.com/sleroq/sloffy/logger"
	"github.com/yitsushi/go-misskey"
	"github.com/yitsushi/go-misskey/core"
	"github.com/yitsushi/go-misskey/models"
	"github.com/yitsushi/go-misskey/services/notes"
)

type options struct {
	MisskeyToken    string
	MisskeyInstance string
}

func getEnv() (options, error) {
	var opts options

	opts.MisskeyToken = os.Getenv("MISSKEY_TOKEN")
	if opts.MisskeyToken == "" {
		return options{}, fmt.Errorf("MISSKEY_TOKEN not provided")
	}
	opts.MisskeyInstance = os.Getenv("MISSKEY_INSTANCE")
	if opts.MisskeyToken == "" {
		return options{}, fmt.Errorf("MISSKEY_INSTANCE not provided")
	}

	return opts, nil
}

func main() {
	log, err := logger.New("sloffy.log")
	if err != nil {
		fmt.Println("Creating new Logger:", err)
		os.Exit(1)
	}

	opts, err := getEnv()
	if err != nil {
		log.Fatalf("Getting env: %v", err)
	}

	err = doSomething(opts, log)
	if err != nil {
		log.Fatalf("doing something: %v", err)
	}
}

type Poll struct {
	Choices      []string
	Multiple     bool
	ExpiresAt    int
	ExpiredAfter int
}

type NoteCreateReq struct {
	Visibility        string   `json:"visability"`
	VisibleUserIds    []string `json:"visibleUserIds"`
	Text              string   `json:"text"`
	Cw                string   `json:"cw"`
	LocalOnly         bool     `json:"localOnly"`
	NoExtractMentions bool
	NoExtractHashtags bool
	NoExtractEmojis   bool
	FileIds           []string
	MediaIds          []string
	ReplyId           string
	RenoteId          string
	ChannelId         string
	Poll
	I string `json:"i"`
}

type NoteGetReq struct {
	Local     bool
	Reply     bool
	Renote    bool
	WithFiles bool
	Poll      bool
	Limit     int
	SinceId   string
	UntilId   string
}

func doSomething(opts options, log *logger.Logger) error {
	client, err := misskey.NewClientWithOptions(misskey.WithSimpleConfig(opts.MisskeyInstance, opts.MisskeyToken))
	if err != nil {
		return fmt.Errorf("creating Misskey client: %w", err)
	}

	response, err := client.Notes().Create(notes.CreateRequest{
		Text:       core.NewString("meow moew"),
		Visibility: models.VisibilityFollowers,
	})

	if err != nil {
		return fmt.Errorf("Creating note: %s", err)
	}

	log.Println(response.CreatedNote.ID)

	return nil
}
