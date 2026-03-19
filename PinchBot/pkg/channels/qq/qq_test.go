package qq

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/openapi/options"

	"github.com/sipeed/pinchbot/pkg/bus"
	"github.com/sipeed/pinchbot/pkg/channels"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/media"
)

func TestHandleC2CMessage_IncludesAccountIDMetadata(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		dedup:       make(map[string]time.Time),
		done:        make(chan struct{}),
		ctx:         context.Background(),
	}

	err := ch.handleC2CMessage()(nil, &dto.WSC2CMessageData{
		ID:      "msg-1",
		Content: "hello",
		Author: &dto.User{
			ID: "7750283E123456",
		},
	})
	if err != nil {
		t.Fatalf("handleC2CMessage() error = %v", err)
	}

	inbound := waitInboundMessage(t, messageBus)
	if inbound.Metadata["account_id"] != "7750283E123456" {
		t.Fatalf("account_id metadata = %q, want %q", inbound.Metadata["account_id"], "7750283E123456")
	}
}

func TestHandleC2CMessage_AttachmentOnlyPublishesMedia(t *testing.T) {
	messageBus := bus.NewMessageBus()
	store := media.NewFileMediaStore()
	localPath := writeTempFile(t, t.TempDir(), "image.png", []byte("fake-image"))

	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		dedup:       make(map[string]time.Time),
		done:        make(chan struct{}),
		ctx:         context.Background(),
		downloadFn: func(urlStr, filename string) string {
			if filename != "image.png" {
				t.Fatalf("download filename = %q, want image.png", filename)
			}
			return localPath
		},
	}
	ch.SetMediaStore(store)

	err := ch.handleC2CMessage()(nil, &dto.WSC2CMessageData{
		ID:      "msg-1",
		Content: "",
		Author: &dto.User{
			ID: "user-1",
		},
		Attachments: []*dto.MessageAttachment{
			{
				URL:         "https://cdn.example.com/image.png",
				FileName:    "image.png",
				ContentType: "image/png",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleC2CMessage() error = %v", err)
	}

	inbound := waitInboundMessage(t, messageBus)
	if inbound.Content != "[image: image.png]" {
		t.Fatalf("content = %q, want %q", inbound.Content, "[image: image.png]")
	}
	if len(inbound.Media) != 1 {
		t.Fatalf("media count = %d, want 1", len(inbound.Media))
	}
	if !strings.HasPrefix(inbound.Media[0], "media://") {
		t.Fatalf("media ref = %q, want media://*", inbound.Media[0])
	}
}

func TestSend_RoutesGroupMessageAndSanitizesURL(t *testing.T) {
	messageBus := bus.NewMessageBus()
	api := &fakeQQAPI{}
	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		api:         api,
		dedup:       make(map[string]time.Time),
		done:        make(chan struct{}),
	}
	ch.SetRunning(true)
	ch.chatType.Store("group-1", "group")

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "group-1",
		Content: "visit https://example.com/path",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if len(api.groupMessages) != 1 {
		t.Fatalf("groupMessages = %d, want 1", len(api.groupMessages))
	}
	msg, ok := api.groupMessages[0].(*dto.MessageToCreate)
	if !ok {
		t.Fatalf("groupMessages[0] type = %T, want *dto.MessageToCreate", api.groupMessages[0])
	}
	if msg.Content != "visit https://example。com/path" {
		t.Fatalf("msg.Content = %q, want sanitized URL", msg.Content)
	}
}

func TestSend_UsesPassiveReplyMetadata(t *testing.T) {
	messageBus := bus.NewMessageBus()
	api := &fakeQQAPI{}
	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		api:         api,
		dedup:       make(map[string]time.Time),
		done:        make(chan struct{}),
	}
	ch.SetRunning(true)
	ch.chatType.Store("group-1", "group")
	ch.lastMsgID.Store("group-1", "msg-1")
	seqCounter := &atomic.Uint64{}
	ch.msgSeqCounters.Store("group-1", seqCounter)

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "group-1",
		Content: "reply content",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	msg, ok := api.groupMessages[0].(*dto.MessageToCreate)
	if !ok {
		t.Fatalf("groupMessages[0] type = %T, want *dto.MessageToCreate", api.groupMessages[0])
	}
	if msg.MsgID != "msg-1" {
		t.Fatalf("msg.MsgID = %q, want msg-1", msg.MsgID)
	}
	if msg.MsgSeq != 1 {
		t.Fatalf("msg.MsgSeq = %d, want 1", msg.MsgSeq)
	}
}

func TestSendMedia_UsesLocalFileUploadForGroup(t *testing.T) {
	messageBus := bus.NewMessageBus()
	store := media.NewFileMediaStore()
	tmpFile := writeTempFile(t, t.TempDir(), "photo.jpg", []byte("image-bytes"))
	ref, err := store.Store(tmpFile, media.MediaMeta{
		Filename:    "photo.jpg",
		ContentType: "image/jpeg",
		Source:      "test",
	}, "qq:test")
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	api := &fakeQQAPI{
		transportResp: mustJSON(t, dto.Message{FileInfo: []byte("uploaded-file-info")}),
	}
	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		api:         api,
		dedup:       make(map[string]time.Time),
		done:        make(chan struct{}),
		ctx:         context.Background(),
	}
	ch.SetRunning(true)
	ch.SetMediaStore(store)
	ch.chatType.Store("group-1", "group")
	ch.lastMsgID.Store("group-1", "msg-1")
	ch.msgSeqCounters.Store("group-1", &atomic.Uint64{})

	err = ch.SendMedia(context.Background(), bus.OutboundMediaMessage{
		ChatID: "group-1",
		Parts: []bus.MediaPart{{
			Type:    "image",
			Ref:     ref,
			Caption: "see https://example.com/image",
		}},
	})
	if err != nil {
		t.Fatalf("SendMedia() error = %v", err)
	}

	if len(api.transportCalls) != 1 {
		t.Fatalf("transportCalls = %d, want 1", len(api.transportCalls))
	}
	upload := api.transportCalls[0]
	if upload.method != "POST" {
		t.Fatalf("upload method = %q, want POST", upload.method)
	}
	if upload.url != "https://api.sgroup.qq.com/v2/groups/group-1/files" {
		t.Fatalf("upload url = %q", upload.url)
	}
	if upload.body.URL != "" {
		t.Fatalf("upload URL = %q, want empty for local file", upload.body.URL)
	}
	if upload.body.FileData == "" {
		t.Fatal("upload file_data should not be empty")
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(upload.body.FileData)
	if decodeErr != nil {
		t.Fatalf("failed to decode file_data: %v", decodeErr)
	}
	if string(decoded) != "image-bytes" {
		t.Fatalf("decoded file_data = %q, want image-bytes", string(decoded))
	}
	if upload.body.FileType != 1 {
		t.Fatalf("upload file_type = %d, want 1", upload.body.FileType)
	}

	if len(api.groupMessages) != 1 {
		t.Fatalf("groupMessages = %d, want 1", len(api.groupMessages))
	}
	msg, ok := api.groupMessages[0].(*dto.MessageToCreate)
	if !ok {
		t.Fatalf("groupMessages[0] type = %T, want *dto.MessageToCreate", api.groupMessages[0])
	}
	if msg.MsgType != dto.RichMediaMsg {
		t.Fatalf("msg.MsgType = %d, want %d", msg.MsgType, dto.RichMediaMsg)
	}
	if msg.MsgID != "msg-1" {
		t.Fatalf("msg.MsgID = %q, want msg-1", msg.MsgID)
	}
	if msg.MsgSeq != 1 {
		t.Fatalf("msg.MsgSeq = %d, want 1", msg.MsgSeq)
	}
	if msg.Content != "see https://example。com/image" {
		t.Fatalf("msg.Content = %q", msg.Content)
	}
	if msg.Media == nil || string(msg.Media.FileInfo) != "uploaded-file-info" {
		t.Fatalf("msg.Media.FileInfo = %q, want uploaded-file-info", string(msg.Media.FileInfo))
	}
}

func TestSendMedia_UsesRemoteURLUploadForC2C(t *testing.T) {
	messageBus := bus.NewMessageBus()
	api := &fakeQQAPI{
		transportResp: mustJSON(t, dto.Message{FileInfo: []byte("remote-file-info")}),
	}
	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		api:         api,
		dedup:       make(map[string]time.Time),
		done:        make(chan struct{}),
		ctx:         context.Background(),
	}
	ch.SetRunning(true)
	ch.chatType.Store("user-1", "direct")

	err := ch.SendMedia(context.Background(), bus.OutboundMediaMessage{
		ChatID: "user-1",
		Parts: []bus.MediaPart{{
			Type: "file",
			Ref:  "https://cdn.example.com/report.pdf",
		}},
	})
	if err != nil {
		t.Fatalf("SendMedia() error = %v", err)
	}

	if len(api.transportCalls) != 1 {
		t.Fatalf("transportCalls = %d, want 1", len(api.transportCalls))
	}
	upload := api.transportCalls[0]
	if upload.url != "https://api.sgroup.qq.com/v2/users/user-1/files" {
		t.Fatalf("upload url = %q", upload.url)
	}
	if upload.body.URL != "https://cdn.example.com/report.pdf" {
		t.Fatalf("upload URL = %q", upload.body.URL)
	}
	if upload.body.FileData != "" {
		t.Fatalf("upload file_data = %q, want empty", upload.body.FileData)
	}
	if upload.body.FileType != 4 {
		t.Fatalf("upload file_type = %d, want 4", upload.body.FileType)
	}

	if len(api.c2cMessages) != 1 {
		t.Fatalf("c2cMessages = %d, want 1", len(api.c2cMessages))
	}
	msg, ok := api.c2cMessages[0].(*dto.MessageToCreate)
	if !ok {
		t.Fatalf("c2cMessages[0] type = %T, want *dto.MessageToCreate", api.c2cMessages[0])
	}
	if msg.MsgType != dto.RichMediaMsg {
		t.Fatalf("msg.MsgType = %d, want %d", msg.MsgType, dto.RichMediaMsg)
	}
	if msg.Media == nil || string(msg.Media.FileInfo) != "remote-file-info" {
		t.Fatalf("msg.Media.FileInfo = %q, want remote-file-info", string(msg.Media.FileInfo))
	}
}

func TestSendMedia_ReturnsSendFailedWithoutMediaStore(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		api:         &fakeQQAPI{},
		dedup:       make(map[string]time.Time),
		done:        make(chan struct{}),
		ctx:         context.Background(),
	}
	ch.SetRunning(true)
	ch.chatType.Store("group-1", "group")

	err := ch.SendMedia(context.Background(), bus.OutboundMediaMessage{
		ChatID: "group-1",
		Parts: []bus.MediaPart{{
			Type: "image",
			Ref:  "media://missing",
		}},
	})
	if !errors.Is(err, channels.ErrSendFailed) {
		t.Fatalf("SendMedia() error = %v, want ErrSendFailed", err)
	}
}

func TestSendMedia_ReturnsSendFailedWhenLocalFileExceedsBase64MiBLimit(t *testing.T) {
	messageBus := bus.NewMessageBus()
	store := media.NewFileMediaStore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "qq-media-too-large-*.bin")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer tmpFile.Close()

	content := make([]byte, bytesPerMiB+1)
	if _, writeErr := tmpFile.Write(content); writeErr != nil {
		t.Fatalf("Write() error = %v", writeErr)
	}

	ref, err := store.Store(tmpFile.Name(), media.MediaMeta{
		Filename:    "large.bin",
		ContentType: "application/octet-stream",
	}, "qq:test")
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	api := &fakeQQAPI{}
	ch := &QQChannel{
		BaseChannel: channels.NewBaseChannel("qq", nil, messageBus, nil),
		config: config.QQConfig{
			MaxBase64FileSizeMiB: 1,
		},
		api:   api,
		dedup: make(map[string]time.Time),
		done:  make(chan struct{}),
		ctx:   context.Background(),
	}
	ch.SetRunning(true)
	ch.SetMediaStore(store)
	ch.chatType.Store("group-1", "group")

	err = ch.SendMedia(context.Background(), bus.OutboundMediaMessage{
		ChatID: "group-1",
		Parts: []bus.MediaPart{{
			Type: "file",
			Ref:  ref,
		}},
	})
	if !errors.Is(err, channels.ErrSendFailed) {
		t.Fatalf("SendMedia() error = %v, want ErrSendFailed", err)
	}
	if len(api.transportCalls) != 0 {
		t.Fatalf("transportCalls = %d, want 0", len(api.transportCalls))
	}
}

type fakeQQAPI struct {
	transportResp  []byte
	transportErr   error
	groupErr       error
	c2cErr         error
	wsErr          error
	transportCalls []fakeTransportCall
	groupMessages  []dto.APIMessage
	c2cMessages    []dto.APIMessage
}

type fakeTransportCall struct {
	method string
	url    string
	body   qqMediaUpload
}

func (f *fakeQQAPI) WS(context.Context, map[string]string, string) (*dto.WebsocketAP, error) {
	if f.wsErr != nil {
		return nil, f.wsErr
	}
	return &dto.WebsocketAP{}, nil
}

func (f *fakeQQAPI) PostGroupMessage(
	_ context.Context,
	_ string,
	msg dto.APIMessage,
	_ ...options.Option,
) (*dto.Message, error) {
	f.groupMessages = append(f.groupMessages, msg)
	return &dto.Message{}, f.groupErr
}

func (f *fakeQQAPI) PostC2CMessage(
	_ context.Context,
	_ string,
	msg dto.APIMessage,
	_ ...options.Option,
) (*dto.Message, error) {
	f.c2cMessages = append(f.c2cMessages, msg)
	return &dto.Message{}, f.c2cErr
}

func (f *fakeQQAPI) Transport(_ context.Context, method, url string, body any) ([]byte, error) {
	upload, ok := body.(*qqMediaUpload)
	if !ok {
		return nil, errors.New("unexpected transport body type")
	}
	f.transportCalls = append(f.transportCalls, fakeTransportCall{
		method: method,
		url:    url,
		body:   *upload,
	})
	return f.transportResp, f.transportErr
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return b
}

func waitInboundMessage(t *testing.T, messageBus *bus.MessageBus) bus.InboundMessage {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	inbound, ok := messageBus.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("timeout waiting for inbound message")
	}
	return inbound
}

func writeTempFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()

	filePath := dir + string(os.PathSeparator) + name
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return filePath
}
