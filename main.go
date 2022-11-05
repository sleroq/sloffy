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
	logger "github.com/sleroq/sloffy/logger"
	"github.com/yitsushi/go-misskey"

	"github.com/yitsushi/go-misskey/models"
	"github.com/yitsushi/go-misskey/services/drive/files"
	"github.com/yitsushi/go-misskey/services/notes"
	"golang.org/x/exp/slices"
)

type options struct {
	MisskeyToken    string
	MisskeyInstance string

	PixivAccessToken  string
	PixivRefreshToken string
}

func getEnv() (opts options, err error) {
	opts.MisskeyToken = os.Getenv("MISSKEY_TOKEN")
	if opts.MisskeyToken == "" {
		return options{}, fmt.Errorf("MISSKEY_TOKEN not provided")
	}
	opts.MisskeyInstance = os.Getenv("MISSKEY_INSTANCE")
	if opts.MisskeyToken == "" {
		return options{}, fmt.Errorf("MISSKEY_INSTANCE not provided")
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
		return nil, 0, fmt.Errorf("Loggin in Pixiv account: %w", err)
	}

	pixivUID, err := strconv.ParseUint(account.ID, 10, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("Could not convert account.ID to uint")
	}

	return pixiv.NewApp(), pixivUID, nil
}

func initMisskey(opts options) (*misskey.Client, error) {
	misskeyClient, err := misskey.NewClientWithOptions(misskey.WithSimpleConfig(opts.MisskeyInstance, opts.MisskeyToken))
	misskeyClient.LogLevel(logrus.DebugLevel)

	if err != nil {
		return nil, fmt.Errorf("Creating Misskey client: %w", err)
	}

	return misskeyClient, nil
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

	pixivClient, pixivUID, err := initPixiv(opts)
	if err != nil {
		log.Fatalf("Initializing pixiv: %v", err)
	}

	misskeyClient, err := initMisskey(opts)
	if err != nil {
		log.Fatalf("Initializing Misskey: %v", err)
	}

	ticker := time.NewTicker(time.Second * 20)
	for ; true; <-ticker.C {
		err := checkAndPost(pixivClient, pixivUID, misskeyClient, log)
		if err != nil {
			log.Printf("ERROR: %v", err)
		}
	}
}

func getOldBookmarks() ([]uint64, error) {
	bookmarksFile := "bookmarks.txt"

	data, err := os.ReadFile(bookmarksFile)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			data = []byte{}
		} else {
			return nil, fmt.Errorf("reading old bookmarks from file: %w", err)
		}
	}

	stringIDs := strings.Split(string(data), "\n")

	var bookmarks []uint64
	for _, id := range stringIDs {
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

func checkAndPost(pixivClient *pixiv.AppPixivAPI, pixivUID uint64, misskeyClient *misskey.Client, log *logger.Logger) error {
	oldBookmarks, err := getOldBookmarks()
	if err != nil {
		return fmt.Errorf("reading old bookmarks from file: %w", err)
	}

	newBookmarks, err := getBookmarks(pixivClient, pixivUID)
	if err != nil {
		return fmt.Errorf("getting new bookmarks: %w", err)
	}

	if len(oldBookmarks) == 0 {
		for _, bookmark := range newBookmarks {
			oldBookmarks = append(oldBookmarks, bookmark.ID)
		}

		err = saveBookmarks(newBookmarks)
		if err != nil {
			return fmt.Errorf("saving bookmarks list for the first time: %w", err)
		}
	}

	var actuallyNew []pixiv.Illust
	for _, bookmark := range newBookmarks {
		if !slices.Contains(oldBookmarks, bookmark.ID) {
			actuallyNew = append(actuallyNew, bookmark)
		}
	}

	if len(actuallyNew) > 0 {
		log.Println("Found", len(actuallyNew), "new bookmarks")

		for _, illust := range actuallyNew {
			illustID := strconv.FormatUint(illust.ID, 10)

			var urls []string
			if illust.MetaSinglePage.OriginalImageURL == "" {
				for _, img := range illust.MetaPages {
					urls = append(urls, img.Images.Medium)
				}
			} else {
				urls = append(urls, illust.MetaSinglePage.OriginalImageURL)
			}

			noteText := fmt.Sprintf("[%s](https://www.pixiv.net/en/artworks/%s)", illust.Title, illustID)
			var uploadedFiles []string

			httpClient := &http.Client{}
			for _, u := range urls {
				image, err := downloadFromPixiv(httpClient, u)
				if err != nil {
					return fmt.Errorf("downloading image %s: %w", u, err)
				}

				file, err := misskeyClient.Drive().File().Create(files.CreateRequest{
					FolderID:    "977okw2d1d", // FIXME
					Name:        filepath.Base(u),
					IsSensitive: false, // FIXME: Get this from pixiv api
					Content:     image,
				})
				if err != nil {
					return fmt.Errorf("saving file ro misskey drive: %w", err)
				}

				uploadedFiles = append(uploadedFiles, file.ID)
			}

			response, err := misskeyClient.Notes().Create(notes.CreateRequest{
				Visibility: models.VisibilityFollowers,
				Text:       &noteText,
				FileIDs:    uploadedFiles,
			})
			if err != nil {
				return fmt.Errorf("Creating note: %s", err)
			}

			log.Println("note for", illustID, "created with id", response.CreatedNote.ID)
		}

		err = saveBookmarks(newBookmarks)
		if err != nil {
			return fmt.Errorf("saving bookmarks list: %w", err)
		}
	} else {
		log.Println("Bookmarks are up to date")
	}

	return nil
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
