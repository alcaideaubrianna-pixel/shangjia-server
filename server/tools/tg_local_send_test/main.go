package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	publishsys "hotgo/addons/youban_publish/logic/sys"
)

const tokenEnv = "YOUBAN_TG_LOCAL_TEST_TOKEN"

type mediaSpec struct {
	Source   string `json:"source"`
	Filename string `json:"filename"`
}

type manifest struct {
	Display []mediaSpec `json:"display"`
	Verify  *mediaSpec  `json:"verify"`
}

type preparedMedia struct {
	path     string
	filename string
	size     int64
	cleanup  func()
}

type options struct {
	endpoint string
	chatID   string
	manifest string
	repeat   int
	timeout  time.Duration
	cleanup  bool
}

func main() {
	opts := parseFlags()
	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "test failed:", err)
		os.Exit(1)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.endpoint, "endpoint", "", "Local Bot API base URL, for example http://127.0.0.1:18081")
	flag.StringVar(&opts.chatID, "chat-id", "", "test channel ID or @username")
	flag.StringVar(&opts.manifest, "manifest", "", "JSON manifest containing display and verify media")
	flag.IntVar(&opts.repeat, "repeat", 1, "number of complete test rounds")
	flag.DurationVar(&opts.timeout, "timeout", 30*time.Minute, "timeout for each Telegram API request")
	flag.BoolVar(&opts.cleanup, "cleanup", true, "delete messages returned by successful test calls")
	flag.Parse()
	return opts
}

func run(parent context.Context, opts options) error {
	if err := validateOptions(opts); err != nil {
		return err
	}
	token := strings.TrimSpace(os.Getenv(tokenEnv))
	if token == "" {
		return fmt.Errorf("environment variable %s is required", tokenEnv)
	}
	spec, err := readManifest(opts.manifest)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "youban-tg-local-test-*")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	downloadClient := &http.Client{Timeout: opts.timeout}
	display, err := prepareAll(parent, downloadClient, tempDir, spec.Display)
	if err != nil {
		return err
	}
	defer cleanupPrepared(display)
	var verify *preparedMedia
	if spec.Verify != nil {
		item, prepareErr := prepareOne(parent, downloadClient, tempDir, len(display), *spec.Verify, "video")
		if prepareErr != nil {
			return prepareErr
		}
		verify = &item
		defer cleanupPrepared([]preparedMedia{item})
	}

	apiClient := &http.Client{Timeout: opts.timeout}
	bot, err := tgbot.New(token, tgbot.WithSkipGetMe(), tgbot.WithServerURL(strings.TrimRight(opts.endpoint, "/")), tgbot.WithHTTPClient(opts.timeout, apiClient))
	if err != nil {
		return fmt.Errorf("create Local Bot API client: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, opts.timeout)
	me, err := bot.GetMe(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("probe Local Bot API endpoint: %w", err)
	}
	fmt.Printf("connected bot=@%s display=%d verify=%t repeat=%d cleanup=%t\n", me.Username, len(display), verify != nil, opts.repeat, opts.cleanup)

	for round := 1; round <= opts.repeat; round++ {
		marker := fmt.Sprintf("youban-local-test-%d-%d", time.Now().UnixNano(), round)
		if err = runRound(parent, bot, opts, display, verify, round, marker); err != nil {
			return err
		}
	}
	return nil
}

func validateOptions(opts options) error {
	parsed, err := url.Parse(strings.TrimSpace(opts.endpoint))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("a valid --endpoint is required")
	}
	if strings.TrimSpace(opts.chatID) == "" {
		return errors.New("--chat-id is required")
	}
	if strings.TrimSpace(opts.manifest) == "" {
		return errors.New("--manifest is required")
	}
	if opts.repeat < 1 || opts.repeat > 10 {
		return errors.New("--repeat must be between 1 and 10")
	}
	if opts.timeout < time.Minute {
		return errors.New("--timeout must be at least 1m")
	}
	return nil
}

func readManifest(path string) (manifest, error) {
	var value manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return value, fmt.Errorf("read manifest: %w", err)
	}
	if err = json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("decode manifest: %w", err)
	}
	if len(value.Display) < 2 || len(value.Display) > 10 {
		return value, errors.New("manifest display must contain 2 to 10 photos")
	}
	for index, item := range value.Display {
		if strings.TrimSpace(item.Source) == "" {
			return value, fmt.Errorf("display[%d].source is required", index)
		}
	}
	if value.Verify != nil && strings.TrimSpace(value.Verify.Source) == "" {
		return value, errors.New("verify.source is required")
	}
	return value, nil
}

func prepareAll(ctx context.Context, client *http.Client, dir string, specs []mediaSpec) ([]preparedMedia, error) {
	items := make([]preparedMedia, 0, len(specs))
	for index, spec := range specs {
		item, err := prepareOne(ctx, client, dir, index, spec, "image")
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func prepareOne(ctx context.Context, client *http.Client, dir string, index int, spec mediaSpec, mediaType string) (preparedMedia, error) {
	filename := safeFilename(spec.Filename, spec.Source, index)
	path := filepath.Join(dir, fmt.Sprintf("%02d-%s", index+1, filename))
	started := time.Now()
	if err := materialize(ctx, client, spec.Source, path); err != nil {
		return preparedMedia{}, fmt.Errorf("prepare %s: %w", filename, err)
	}
	processedPath, cleanup, err := publishsys.PrepareLocalBotAPITestMedia(ctx, path, mediaType)
	if err != nil {
		return preparedMedia{}, fmt.Errorf("normalize %s: %w", filename, err)
	}
	info, err := os.Stat(processedPath)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return preparedMedia{}, fmt.Errorf("stat %s: %w", filename, err)
	}
	if mediaType == "video" {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".mp4"
	} else if !strings.EqualFold(filepath.Ext(processedPath), filepath.Ext(filename)) {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + filepath.Ext(processedPath)
	}
	fmt.Printf("prepared file=%s bytes=%d duration=%s\n", filename, info.Size(), time.Since(started).Round(time.Millisecond))
	return preparedMedia{path: processedPath, filename: filename, size: info.Size(), cleanup: cleanup}, nil
}

func materialize(ctx context.Context, client *http.Client, source, target string) error {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
		if requestErr != nil {
			return requestErr
		}
		res, requestErr := client.Do(req)
		if requestErr != nil {
			return requestErr
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("download returned HTTP %d", res.StatusCode)
		}
		return writeFile(target, res.Body)
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	return writeFile(target, sourceFile)
}

func writeFile(target string, source io.Reader) error {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	return errors.Join(copyErr, closeErr)
}

func safeFilename(explicit, source string, index int) string {
	name := filepath.Base(strings.TrimSpace(explicit))
	if name == "." || name == "" {
		if parsed, err := url.Parse(source); err == nil {
			name = filepath.Base(parsed.Path)
		}
	}
	if name == "." || name == "/" || name == "" {
		name = "media-" + strconv.Itoa(index+1)
	}
	return strings.ReplaceAll(name, string(filepath.Separator), "-")
}

func runRound(parent context.Context, bot *tgbot.Bot, opts options, display []preparedMedia, verify *preparedMedia, round int, marker string) error {
	started := time.Now()
	displayMessages, err := sendAlbum(parent, bot, opts, display, marker)
	if err != nil {
		return fmt.Errorf("round %d display: %w", round, err)
	}
	messageIDs := messageIDs(displayMessages)
	if len(displayMessages) != len(display) {
		return fmt.Errorf("round %d display incomplete: expected=%d actual=%d messageIds=%v", round, len(display), len(displayMessages), messageIDs)
	}
	if verify != nil {
		message, sendErr := sendVideo(parent, bot, opts, *verify, marker)
		if sendErr != nil {
			return fmt.Errorf("round %d verify: %w", round, sendErr)
		}
		if message == nil || message.ID <= 0 {
			return fmt.Errorf("round %d verify returned no message", round)
		}
		messageIDs = append(messageIDs, message.ID)
	}
	fmt.Printf("round=%d status=success messages=%d ids=%v duration=%s\n", round, len(messageIDs), messageIDs, time.Since(started).Round(time.Millisecond))
	if opts.cleanup {
		if err = deleteMessages(parent, bot, opts, messageIDs); err != nil {
			return fmt.Errorf("round %d cleanup: %w", round, err)
		}
		fmt.Printf("round=%d cleanup=success deleted=%d\n", round, len(messageIDs))
	}
	return nil
}

func sendAlbum(parent context.Context, bot *tgbot.Bot, opts options, media []preparedMedia, marker string) ([]*models.Message, error) {
	inputs := make([]models.InputMedia, 0, len(media))
	files := make([]*os.File, 0, len(media))
	for index, item := range media {
		file, err := os.Open(item.path)
		if err != nil {
			closeFiles(files)
			return nil, err
		}
		files = append(files, file)
		caption := ""
		if index == 0 {
			caption = marker
		}
		inputs = append(inputs, &models.InputMediaPhoto{
			Media:           "attach://photo_" + strconv.Itoa(index),
			Caption:         caption,
			MediaAttachment: file,
		})
	}
	defer closeFiles(files)
	ctx, cancel := context.WithTimeout(parent, opts.timeout)
	defer cancel()
	started := time.Now()
	result, err := bot.SendMediaGroup(ctx, &tgbot.SendMediaGroupParams{ChatID: opts.chatID, Media: inputs})
	fmt.Printf("sendMediaGroup expected=%d actual=%d bytes=%d duration=%s err=%v\n", len(media), len(result), totalSize(media), time.Since(started).Round(time.Millisecond), err)
	return result, err
}

func sendVideo(parent context.Context, bot *tgbot.Bot, opts options, media preparedMedia, marker string) (*models.Message, error) {
	file, err := os.Open(media.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(parent, opts.timeout)
	defer cancel()
	started := time.Now()
	result, err := bot.SendVideo(ctx, &tgbot.SendVideoParams{
		ChatID:            opts.chatID,
		Video:             &models.InputFileUpload{Filename: media.filename, Data: file},
		Caption:           marker + " verify",
		SupportsStreaming: true,
	})
	fmt.Printf("sendVideo bytes=%d returned=%t duration=%s err=%v\n", media.size, result != nil, time.Since(started).Round(time.Millisecond), err)
	return result, err
}

func deleteMessages(parent context.Context, bot *tgbot.Bot, opts options, ids []int) error {
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(parent, opts.timeout)
		_, err := bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: opts.chatID, MessageID: id})
		cancel()
		if err != nil {
			return fmt.Errorf("delete message %d: %w", id, err)
		}
	}
	return nil
}

func messageIDs(messages []*models.Message) []int {
	ids := make([]int, 0, len(messages))
	for _, message := range messages {
		if message != nil && message.ID > 0 {
			ids = append(ids, message.ID)
		}
	}
	return ids
}

func totalSize(media []preparedMedia) int64 {
	var total int64
	for _, item := range media {
		total += item.size
	}
	return total
}

func closeFiles(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

func cleanupPrepared(items []preparedMedia) {
	for _, item := range items {
		if item.cleanup != nil {
			item.cleanup()
		}
	}
}
