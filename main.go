package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/everpcpc/pixiv"
	"github.com/sirupsen/logrus"
	"github.com/yitsushi/go-misskey"

	"github.com/yitsushi/go-misskey/models"
	"github.com/yitsushi/go-misskey/services/drive/files"
	"github.com/yitsushi/go-misskey/services/notes"
	"golang.org/x/exp/slices"
)

type options struct {
	MisskeyToken    string
	MisskeyInstance string
	MisskeyFolderID string

	PixivAccessToken  string
	PixivRefreshToken string
}

var log = logrus.New()

func getEnv() (opts options, err error) {
	opts.MisskeyToken = os.Getenv("MISSKEY_TOKEN")
	if opts.MisskeyToken == "" {
		return options{}, fmt.Errorf("MISSKEY_TOKEN not provided")
	}
	opts.MisskeyInstance = os.Getenv("MISSKEY_INSTANCE")
	if opts.MisskeyToken == "" {
		return options{}, fmt.Errorf("MISSKEY_INSTANCE not provided")
	}
	opts.MisskeyFolderID = os.Getenv("MISSKEY_FOLDER_ID")
	if opts.MisskeyFolderID == "" {
		return options{}, fmt.Errorf("MISSKEY_FOLDER_ID not provided")
	}

	opts.PixivAccessToken = os.Getenv("PIXIV_ACCESS_TOKEN")
	if opts.PixivAccessToken == "" {
		return options{}, fmt.Errorf("PIXIV_ACCESS_TOKEN not provided")
	}
	opts.PixivRefreshToken = os.Getenv("PIXIV_REFRESH_TOKEN")
	if opts.PixivRefreshToken == "" {
		return options{}, fmt.Errorf("PIXIV_REFRESH_TOKEN not provided")
	}

	return opts, nil
}

func initPixiv(opts options) (*pixiv.AppPixivAPI, uint64, error) {
	account, err := pixiv.LoadAuth(opts.PixivAccessToken, opts.PixivRefreshToken, time.Now().Add(time.Millisecond*3000))
	if err != nil {
		return nil, 0, fmt.Errorf("loggin in Pixiv account: %w", err)
	}

	pixivUID, err := strconv.ParseUint(account.ID, 10, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("could not convert account.ID to uint")
	}

	return pixiv.NewApp(), pixivUID, nil
}

func initMisskey(opts options) (*misskey.Client, error) {
	misskeyClient, err := misskey.NewClientWithOptions(misskey.WithSimpleConfig(opts.MisskeyInstance, opts.MisskeyToken))
	// misskeyClient.LogLevel(logrus.WarnLevel)

	if err != nil {
		return nil, fmt.Errorf("creating Misskey client: %w", err)
	}

	return misskeyClient, nil
}

func main() {
	file, err := os.OpenFile("sloffy.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.Out = file
	} else {
		log.Info("Failed to log to file, using default stderr")
	}

	opts, err := getEnv()
	if err != nil {
		log.Fatalf("Getting env: %v", err)
	}

	pixivClient, pixivUID, err := initPixiv(opts)
	if err != nil {
		log.Fatalf("Initializing pixiv: %v", err)
	}

	misskeyClient, err := initMisskey(opts)
	if err != nil {
		log.Fatalf("Initializing Misskey: %v", err)
	}

	errorCounter := 0
	ticker := time.NewTicker(time.Minute)
	for ; true; <-ticker.C {
		err := checkAndPost(pixivClient, pixivUID, misskeyClient, opts.MisskeyFolderID)
		if err != nil {
			if errorCounter > 10 {
				log.Fatalln("Too many errors, last one:", err)
			}

			log.Errorf("checkAndPost failed: %v", err)
			errorCounter += 1
			continue
		}
		errorCounter = 0
	}
}

func getOldBookmarks() ([]uint64, error) {
	bookmarksFile := "bookmarks.txt"

	data, err := os.ReadFile(bookmarksFile)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return []uint64{}, nil
		} else {
			return nil, fmt.Errorf("reading old bookmarks from file: %w", err)
		}
	}

	stringIDs := strings.Split(string(data), "\n")

	var bookmarks []uint64
	for _, id := range stringIDs {
		if id == "" {
			continue
		}
		parsedID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing id to uint \"%s\": %w", id, err)
		}
		bookmarks = append(bookmarks, parsedID)
	}

	return bookmarks, nil
}

func saveBookmarks(bookmarks []pixiv.Illust) error {
	bookmarksFile := "bookmarks.txt"

	var ids []string
	for _, illust := range bookmarks {
		ids = append(ids, strconv.FormatUint(illust.ID, 10))
	}

	err := os.WriteFile(bookmarksFile, []byte(strings.Join(ids, "\n")), 0644)
	if err != nil {
		return fmt.Errorf("saving bookmarks to file: %w", err)
	}

	return nil
}

func getBookmarks(pixivClient *pixiv.AppPixivAPI, uid uint64) ([]pixiv.Illust, error) {
	bookmarks, _, err := pixivClient.UserBookmarksIllust(uid, "public", 0, "")
	if err != nil {
		return nil, fmt.Errorf("getting bookmarks: %w", err)
	}

	return bookmarks, nil
}

func downloadFromPixiv(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Referer", "https://app-api.pixiv.net/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %s", err)
	}

	return result, nil
}

func checkAndPost(pixivClient *pixiv.AppPixivAPI, pixivUID uint64, misskeyClient *misskey.Client, folderID string) error {
	oldBookmarks, err := getOldBookmarks()
	if err != nil {
		return fmt.Errorf("reading old bookmarks from file: %w", err)
	}

	newBookmarks, err := getBookmarks(pixivClient, pixivUID)
	if err != nil {
		return fmt.Errorf("getting new bookmarks: %w", err)
	}

	// If this is first run ever
	if len(oldBookmarks) == 0 {
		for _, bookmark := range newBookmarks {
			oldBookmarks = append(oldBookmarks, bookmark.ID)
		}
	}

	// Using only first 20 bookmarks
	// because if user removes some illustations from favorites
	// some old bookmarks now can fit in the last 30 bookmarks
	// but they aren't present in cached bookmarks.txt from last runs
	var someNewBookmarks []pixiv.Illust
	for index, illust := range newBookmarks {
		someNewBookmarks = append(someNewBookmarks, illust)
		if index > 20 {
			break
		}
	}

	// Looking for new illustations
	var actuallyNew []pixiv.Illust
	for _, bookmark := range someNewBookmarks {
		if !slices.Contains(oldBookmarks, bookmark.ID) {
			actuallyNew = append(actuallyNew, bookmark)
		}
	}

	if len(actuallyNew) > 0 {
		log.Debugln("Found", len(actuallyNew), "new bookmarks")
	}

	for _, illust := range actuallyNew {
		illustID := strconv.FormatUint(illust.ID, 10)

		var urls []string
		if illust.MetaSinglePage.OriginalImageURL == "" {
			for _, img := range illust.MetaPages {
				urls = append(urls, img.Images.Large)
			}
		} else {
			urls = append(urls, illust.Images.Large)
		}

		noteText := fmt.Sprintf("[%s](https://www.pixiv.net/en/artworks/%s)\n#art #pixiv", illust.Title, illustID)
		var uploadedFiles []string

		httpClient := &http.Client{}
		for index, u := range urls {
			image, err := downloadFromPixiv(httpClient, u)
			if err != nil {
				return fmt.Errorf("downloading image %s: %w", u, err)
			}

			file, err := misskeyClient.Drive().File().Create(files.CreateRequest{
				FolderID:    folderID,
				Name:        filepath.Base(u),
				IsSensitive: illust.XRestrict != 0,
				Content:     image,
			})
			if err != nil {
				return fmt.Errorf("saving file ro misskey drive: %w", err)
			}

			uploadedFiles = append(uploadedFiles, file.ID)

			if index == 16 {
				log.Warn("Misskey attachments limt is reached ;c")
				break
			}
		}

		response, err := misskeyClient.Notes().Create(notes.CreateRequest{
			Visibility: models.VisibilityPublic,
			Text:       &noteText,
			FileIDs:    uploadedFiles,
		})
		if err != nil {
			return fmt.Errorf("creating note: %s", err)
		}

		log.Debugln("note for", illustID, "created with id", response.CreatedNote.ID)
	}

	err = saveBookmarks(newBookmarks)
	if err != nil {
		return fmt.Errorf("saving bookmarks list: %w", err)
	}

	return nil
}
