package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ebike-battery-backend/internal/ds"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const testMediaBaseURL = "http://localhost:9000/ebike-motor-media"

var (
	webmBytes = []byte("\x1A\x45\xDF\xA3\x9F\x42\x86\x81\x01\x42\xF7\x81\x01\x42\xF2\x81\x04webm")
	jpegBytes = []byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")
	mp4Bytes  = []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
)

type testServer struct {
	router     *gin.Engine
	motorModes *fakeMotorModes
	storage    *fakeStorage
	riders     *fakeRiders
}

func newTestServer() *testServer {
	gin.SetMode(gin.TestMode)
	server := &testServer{motorModes: newFakeMotorModes(), storage: newFakeStorage(), riders: newFakeRiders()}
	server.router = gin.New()
	server.router.HandleMethodNotAllowed = true
	RegisterRoutes(server.router,
		NewMotorModeHandler(server.motorModes, server.storage, testMediaBaseURL),
		NewRiderHandler(server.riders))
	server.router.NoRoute(NoRoute(func(c *gin.Context) { c.String(http.StatusNotFound, "ssr") }))
	server.router.NoMethod(NoMethod(func(c *gin.Context) { c.String(http.StatusNotFound, "ssr") }))
	return server
}

func (s *testServer) do(method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	switch value := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(value))
	default:
		encoded, _ := json.Marshal(value)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, request)
	return recorder
}

type uploadFile struct {
	field, name string
	content     []byte
}

func (s *testServer) upload(fields map[string]string, files ...uploadFile) *httptest.ResponseRecorder {
	return s.uploadWithContext(context.Background(), fields, files...)
}

func (s *testServer) uploadWithContext(ctx context.Context, fields map[string]string, files ...uploadFile) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		_ = writer.WriteField(name, value)
	}
	for _, file := range files {
		part, _ := writer.CreateFormFile(file.field, file.name)
		_, _ = part.Write(file.content)
	}
	_ = writer.Close()
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/motor_modes", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, request)
	return recorder
}

func decode[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("ответ не JSON: %v, тело %s", err, recorder.Body.String())
	}
	return value
}

func expectStatus(t *testing.T, recorder *httptest.ResponseRecorder, status int) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("код %d, ожидался %d, тело %s", recorder.Code, status, recorder.Body.String())
	}
}

type listResponse struct {
	MotorModes []struct {
		ID                    uint    `json:"id"`
		ModeName              string  `json:"mode_name"`
		ImageURL              *string `json:"image_url"`
		LikeCount             int     `json:"like_count"`
		CreatedByCurrentRider int     `json:"created_by_current_rider"`
	} `json:"motor_modes"`
	MaxConsumptionWhPerKm float64 `json:"max_consumption_wh_per_km"`
	ConsumptionMin        float64 `json:"consumption_min"`
	ConsumptionMax        float64 `json:"consumption_max"`
}

type detailResponse struct {
	ID                    uint     `json:"id"`
	ModeName              string   `json:"mode_name"`
	ShortDescription      *string  `json:"short_description"`
	Status                string   `json:"status"`
	ImageURL              *string  `json:"image_url"`
	VideoURL              *string  `json:"video_url"`
	SupportPercent        *int     `json:"support_percent"`
	ConsumptionWhPerKm    *float64 `json:"consumption_wh_per_km"`
	RangeKm               int      `json:"range_km"`
	PublishedAt           *string  `json:"published_at"`
	CreatorRiderID        uint     `json:"creator_rider_id"`
	CreatedByCurrentRider int      `json:"created_by_current_rider"`
	LikeCount             int      `json:"like_count"`
	LikedByCurrentRider   int      `json:"liked_by_current_rider"`
}

type messageResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func TestListMotorModesMarksOwnModesAndCountsLikes(t *testing.T) {
	server := newTestServer()

	recorder := server.do(http.MethodGet, "/api/motor_modes", nil)
	expectStatus(t, recorder, http.StatusOK)
	list := decode[listResponse](t, recorder)

	if len(list.MotorModes) != 2 {
		t.Fatalf("ожидались 2 опубликованных режима, пришло %d", len(list.MotorModes))
	}
	eco, race := list.MotorModes[0], list.MotorModes[1]
	if eco.ModeName != "Eco" || eco.CreatedByCurrentRider != 0 || eco.LikeCount != 2 {
		t.Errorf("Eco: %+v", eco)
	}
	if race.ModeName != "Race" || race.CreatedByCurrentRider != 1 || race.LikeCount != 1 {
		t.Errorf("Race: %+v", race)
	}
	if eco.ImageURL == nil || *eco.ImageURL != testMediaBaseURL+"/eco_mode.jpg" {
		t.Errorf("image_url Eco: %v", eco.ImageURL)
	}
	if list.ConsumptionMin != 5.2 || list.ConsumptionMax != 19 || list.MaxConsumptionWhPerKm != 19 {
		t.Errorf("границы слайдера: %+v", list)
	}
}

func TestListMotorModesFiltersByConsumption(t *testing.T) {
	server := newTestServer()

	recorder := server.do(http.MethodGet, "/api/motor_modes?max_consumption_wh_per_km=10", nil)
	expectStatus(t, recorder, http.StatusOK)
	list := decode[listResponse](t, recorder)
	if len(list.MotorModes) != 1 || list.MotorModes[0].ModeName != "Eco" || list.MaxConsumptionWhPerKm != 10 {
		t.Fatalf("фильтр 10 Вт·ч/км: %+v", list)
	}

	for _, bad := range []string{"abc", "-1", "0", "NaN", "0.04", "999.95", "1000", "1e308", "Inf"} {
		recorder = server.do(http.MethodGet, "/api/motor_modes?max_consumption_wh_per_km="+bad, nil)
		expectStatus(t, recorder, http.StatusBadRequest)
		if decode[messageResponse](t, recorder).Status != "fail" {
			t.Errorf("фильтр %q: нет status fail", bad)
		}
	}
}

func TestFeedReturnsPublishedModes(t *testing.T) {
	server := newTestServer()

	first := decode[detailResponse](t, server.do(http.MethodGet, "/api/motor_modes/feed", nil))
	if first.ID != 1 || first.LikeCount != 2 || first.LikedByCurrentRider != 0 || first.RangeKm != 153 {
		t.Errorf("лента без id: %+v", first)
	}
	if first.VideoURL == nil || *first.VideoURL != testMediaBaseURL+"/eco_mode.mp4" {
		t.Errorf("video_url: %v", first.VideoURL)
	}

	next := decode[detailResponse](t, server.do(http.MethodGet, "/api/motor_modes/feed/1?next=true", nil))
	if next.ID != 2 || next.LikedByCurrentRider != 1 || next.CreatedByCurrentRider != 1 {
		t.Errorf("следующий после 1: %+v", next)
	}

	wrapped := decode[detailResponse](t, server.do(http.MethodGet, "/api/motor_modes/feed/2?next=true", nil))
	if wrapped.ID != 1 {
		t.Errorf("после последнего лента идёт по кругу, пришёл %d", wrapped.ID)
	}

	byID := decode[detailResponse](t, server.do(http.MethodGet, "/api/motor_modes/feed/2", nil))
	if byID.ID != 2 {
		t.Errorf("лента по id 2: %+v", byID)
	}
}

func TestFeedHidesDraftsAndDeletedModes(t *testing.T) {
	server := newTestServer()

	expectStatus(t, server.do(http.MethodGet, "/api/motor_modes/feed/3", nil), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodGet, "/api/motor_modes/feed/4", nil), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodGet, "/api/motor_modes/feed/999", nil), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodGet, "/api/motor_modes/feed/abc", nil), http.StatusBadRequest)
}

func TestDraftIsNotFoundBeforeCreation(t *testing.T) {
	server := newTestServer()

	recorder := server.do(http.MethodGet, "/api/motor_modes/draft", nil)
	expectStatus(t, recorder, http.StatusNotFound)
	if decode[messageResponse](t, recorder).Message == "" {
		t.Error("нет текста ошибки")
	}
}

func TestCreateMotorModeUploadsFilesToStorage(t *testing.T) {
	server := newTestServer()

	recorder := server.upload(map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "image", name: "фото.jpeg", content: jpegBytes},
		uploadFile{field: "video", name: "видео.mp4", content: mp4Bytes})
	expectStatus(t, recorder, http.StatusCreated)
	created := decode[detailResponse](t, recorder)

	if created.ID != 5 || created.Status != "draft" || created.CreatorRiderID != 1 || created.ModeName != "Walk Assist" {
		t.Fatalf("созданный черновик: %+v", created)
	}
	if created.ImageURL == nil || *created.ImageURL != testMediaBaseURL+"/motor_mode_5.jpg" {
		t.Errorf("image_url: %v", created.ImageURL)
	}
	if created.VideoURL == nil || *created.VideoURL != testMediaBaseURL+"/motor_mode_5.mp4" {
		t.Errorf("video_url: %v", created.VideoURL)
	}
	image, ok := server.storage.objects["motor_mode_5.jpg"]
	if !ok || image.contentType != "image/jpeg" || !bytes.Equal(image.content, jpegBytes) {
		t.Errorf("фото в хранилище: %+v", image)
	}
	video, ok := server.storage.objects["motor_mode_5.mp4"]
	if !ok || video.contentType != "video/mp4" || !bytes.Equal(video.content, mp4Bytes) {
		t.Errorf("видео в хранилище: %+v", video)
	}
	if key := server.motorModes.modes[5].ImageKey; key == nil || *key != "motor_mode_5.jpg" {
		t.Errorf("image_key в БД: %v", key)
	}

	draft := decode[detailResponse](t, server.do(http.MethodGet, "/api/motor_modes/draft", nil))
	if draft.ID != 5 {
		t.Errorf("черновик после создания: %+v", draft)
	}
}

func TestCreateMotorModeWithoutFilesKeepsMediaEmpty(t *testing.T) {
	server := newTestServer()

	recorder := server.upload(map[string]string{"mode_name": "Walk Assist"})
	expectStatus(t, recorder, http.StatusCreated)
	created := decode[detailResponse](t, recorder)
	if created.ImageURL != nil || created.VideoURL != nil {
		t.Errorf("без файлов url должны быть null: %+v", created)
	}
}

func TestCreateMotorModeRejectsSecondDraft(t *testing.T) {
	server := newTestServer()

	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist"}), http.StatusCreated)
	expectStatus(t, server.upload(map[string]string{"mode_name": "Ещё один"}), http.StatusConflict)
}

func TestCreateMotorModeValidatesInput(t *testing.T) {
	server := newTestServer()

	expectStatus(t, server.upload(map[string]string{"mode_name": "   "}), http.StatusBadRequest)
	expectStatus(t, server.upload(map[string]string{"mode_name": strings.Repeat("я", 51)}), http.StatusBadRequest)
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "image", name: "note.jpg", content: []byte("это не картинка")}), http.StatusBadRequest)
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "video", name: "clip.mp4", content: jpegBytes}), http.StatusBadRequest)
	recorder := server.upload(map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "video", name: "clip.webm", content: webmBytes})
	expectStatus(t, recorder, http.StatusBadRequest)
	if decode[messageResponse](t, recorder).Message != videoRule.wrongType {
		t.Errorf("WebM должен отклоняться по типу: %s", recorder.Body.String())
	}
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist", "image": "фото.jpg"}), http.StatusBadRequest)
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk\x00Assist"}), http.StatusBadRequest)
	expectStatus(t, server.do(http.MethodPost, "/api/motor_modes", map[string]string{"mode_name": "JSON"}), http.StatusUnsupportedMediaType)

	if len(server.storage.objects) != 0 {
		t.Errorf("при ошибке ничего не должно попасть в хранилище: %v", server.storage.objects)
	}
	if _, ok := server.motorModes.modes[5]; ok {
		t.Error("при ошибке черновик не должен создаваться")
	}
}

func TestCreateMotorModeRollsBackWhenStorageFails(t *testing.T) {
	server := newTestServer()
	server.storage.failPut = true

	recorder := server.upload(map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "image", name: "photo.jpg", content: jpegBytes})
	expectStatus(t, recorder, http.StatusBadGateway)
	if _, ok := server.motorModes.modes[5]; ok {
		t.Error("черновик не должен остаться без файлов")
	}
	if len(server.storage.objects) != 0 {
		t.Errorf("хранилище должно остаться пустым: %v", server.storage.objects)
	}
}

func TestCreateMotorModeRemovesImageWhenVideoUploadFails(t *testing.T) {
	server := newTestServer()
	server.storage.failOnSuffix = ".mp4"

	recorder := server.upload(map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "image", name: "photo.jpg", content: jpegBytes},
		uploadFile{field: "video", name: "clip.mp4", content: mp4Bytes})
	expectStatus(t, recorder, http.StatusBadGateway)
	if len(server.storage.objects) != 0 {
		t.Errorf("уже загруженное фото должно удалиться: %v", server.storage.objects)
	}
	if _, ok := server.motorModes.modes[5]; ok {
		t.Error("черновик не должен создаваться")
	}
}

func TestCreateMotorModeCleansUpWhenClientDisconnects(t *testing.T) {
	server := newTestServer()
	ctx, cancel := context.WithCancel(context.Background())
	server.storage.afterPut = func(key string) {
		if strings.HasSuffix(key, ".jpg") {
			cancel()
		}
	}

	server.uploadWithContext(ctx, map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "image", name: "photo.jpg", content: jpegBytes},
		uploadFile{field: "video", name: "clip.mp4", content: mp4Bytes})
	if len(server.storage.objects) != 0 {
		t.Errorf("после обрыва запроса фото должно удалиться из хранилища: %v", server.storage.objects)
	}
	if _, ok := server.motorModes.modes[5]; ok {
		t.Error("черновик не должен создаваться")
	}
}

func TestCreateMotorModeRejectsBrokenForm(t *testing.T) {
	server := newTestServer()

	request := httptest.NewRequest(http.MethodPost, "/api/motor_modes", strings.NewReader("не форма"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	recorder := httptest.NewRecorder()
	server.router.ServeHTTP(recorder, request)
	expectStatus(t, recorder, http.StatusBadRequest)
}

func TestCreateMotorModeLimitsBodySize(t *testing.T) {
	server := newTestServer()
	defer func(limit int64) { maxCreateBodySize = limit }(maxCreateBodySize)
	maxCreateBodySize = 1024

	recorder := server.upload(map[string]string{"mode_name": "Walk Assist"},
		uploadFile{field: "image", name: "photo.jpg", content: append(append([]byte{}, jpegBytes...), make([]byte, 4096)...)})
	expectStatus(t, recorder, http.StatusRequestEntityTooLarge)
	if len(server.storage.objects) != 0 {
		t.Errorf("слишком большой запрос не должен попасть в хранилище: %v", server.storage.objects)
	}
}

func publishBody() map[string]any {
	return map[string]any{
		"short_description":     "Помощь при ведении велосипеда до 6 км/ч",
		"support_percent":       100,
		"consumption_wh_per_km": 3.44,
	}
}

func TestPublishOwnDraft(t *testing.T) {
	server := newTestServer()
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist"}), http.StatusCreated)

	body := publishBody()
	body["status"] = "deleted"
	body["creator_rider_id"] = 7
	body["published_at"] = "2020-01-01T00:00:00Z"
	recorder := server.do(http.MethodPut, "/api/motor_modes/5/publish", body)
	expectStatus(t, recorder, http.StatusOK)
	published := decode[detailResponse](t, recorder)

	if published.Status != "published" || published.CreatorRiderID != 1 || published.PublishedAt == nil ||
		strings.HasPrefix(*published.PublishedAt, "2020") {
		t.Fatalf("системные поля задаёт сервер: %+v", published)
	}
	if *published.SupportPercent != 100 || *published.ConsumptionWhPerKm != 3.4 || published.ModeName != "Walk Assist" {
		t.Errorf("поля по теме: %+v", published)
	}
	expectStatus(t, server.do(http.MethodGet, "/api/motor_modes/draft", nil), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodPut, "/api/motor_modes/5/publish", publishBody()), http.StatusConflict)
}

func TestPublishRenamesDraftWhenNameGiven(t *testing.T) {
	server := newTestServer()
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist"}), http.StatusCreated)

	body := publishBody()
	body["mode_name"] = "Walk"
	published := decode[detailResponse](t, server.do(http.MethodPut, "/api/motor_modes/5/publish", body))
	if published.ModeName != "Walk" {
		t.Errorf("название после публикации: %q", published.ModeName)
	}
}

func TestPublishChecksOwnerAndStatus(t *testing.T) {
	server := newTestServer()

	expectStatus(t, server.do(http.MethodPut, "/api/motor_modes/3/publish", publishBody()), http.StatusForbidden)
	expectStatus(t, server.do(http.MethodPut, "/api/motor_modes/2/publish", publishBody()), http.StatusConflict)
	expectStatus(t, server.do(http.MethodPut, "/api/motor_modes/4/publish", publishBody()), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodPut, "/api/motor_modes/abc/publish", publishBody()), http.StatusBadRequest)
}

func TestPublishValidatesFields(t *testing.T) {
	server := newTestServer()
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist"}), http.StatusCreated)

	for name, change := range map[string]func(map[string]any){
		"без описания":          func(b map[string]any) { delete(b, "short_description") },
		"без поддержки":         func(b map[string]any) { delete(b, "support_percent") },
		"поддержка больше 1000": func(b map[string]any) { b["support_percent"] = 1001 },
		"без расхода":           func(b map[string]any) { delete(b, "consumption_wh_per_km") },
		"расход ноль":           func(b map[string]any) { b["consumption_wh_per_km"] = 0.04 },
		"пустое название":       func(b map[string]any) { b["mode_name"] = " " },
		"нулевой байт":          func(b map[string]any) { b["short_description"] = "до\x00после" },
	} {
		body := publishBody()
		change(body)
		recorder := server.do(http.MethodPut, "/api/motor_modes/5/publish", body)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: код %d", name, recorder.Code)
		}
	}
	expectStatus(t, server.do(http.MethodPut, "/api/motor_modes/5/publish", "{не json"), http.StatusBadRequest)
	if server.motorModes.modes[5].Status != ds.StatusDraft {
		t.Error("после ошибок режим должен остаться черновиком")
	}
}

func TestDeleteOnlyOwnModes(t *testing.T) {
	server := newTestServer()

	expectStatus(t, server.do(http.MethodDelete, "/api/motor_modes/1", nil), http.StatusForbidden)
	expectStatus(t, server.do(http.MethodDelete, "/api/motor_modes/3", nil), http.StatusForbidden)

	recorder := server.do(http.MethodDelete, "/api/motor_modes/2", nil)
	expectStatus(t, recorder, http.StatusOK)
	deleted := decode[map[string]any](t, recorder)
	if deleted["status"] != "deleted" || server.motorModes.modes[2].Status != ds.StatusDeleted {
		t.Errorf("удаление Race: %v", deleted)
	}

	expectStatus(t, server.do(http.MethodDelete, "/api/motor_modes/2", nil), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodDelete, "/api/motor_modes/4", nil), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodDelete, "/api/motor_modes/abc", nil), http.StatusBadRequest)
	expectStatus(t, server.do(http.MethodGet, "/api/motor_modes/feed/2", nil), http.StatusNotFound)
}

func TestDeleteOwnDraft(t *testing.T) {
	server := newTestServer()
	expectStatus(t, server.upload(map[string]string{"mode_name": "Walk Assist"}), http.StatusCreated)

	expectStatus(t, server.do(http.MethodDelete, "/api/motor_modes/5", nil), http.StatusOK)
	expectStatus(t, server.do(http.MethodGet, "/api/motor_modes/draft", nil), http.StatusNotFound)
}

type likeResponse struct {
	MotorModeID         uint `json:"motor_mode_id"`
	LikeCount           int  `json:"like_count"`
	LikedByCurrentRider int  `json:"liked_by_current_rider"`
}

func TestLikeAndUnlike(t *testing.T) {
	server := newTestServer()

	liked := decode[likeResponse](t, server.do(http.MethodPost, "/api/motor_modes/1/like", map[string]int{"like": 1}))
	if liked.MotorModeID != 1 || liked.LikeCount != 3 || liked.LikedByCurrentRider != 1 {
		t.Errorf("лайк Eco: %+v", liked)
	}
	again := decode[likeResponse](t, server.do(http.MethodPost, "/api/motor_modes/1/like", map[string]int{"like": 1}))
	if again.LikeCount != 3 {
		t.Errorf("повторный лайк не добавляет строку: %+v", again)
	}
	unliked := decode[likeResponse](t, server.do(http.MethodPost, "/api/motor_modes/2/like", map[string]int{"like": 0}))
	if unliked.LikeCount != 0 || unliked.LikedByCurrentRider != 0 {
		t.Errorf("отмена лайка Race: %+v", unliked)
	}
}

func TestLikeValidatesInput(t *testing.T) {
	server := newTestServer()

	expectStatus(t, server.do(http.MethodPost, "/api/motor_modes/1/like", map[string]int{"like": 5}), http.StatusBadRequest)
	expectStatus(t, server.do(http.MethodPost, "/api/motor_modes/1/like", map[string]int{}), http.StatusBadRequest)
	expectStatus(t, server.do(http.MethodPost, "/api/motor_modes/3/like", map[string]int{"like": 1}), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodPost, "/api/motor_modes/4/like", map[string]int{"like": 1}), http.StatusNotFound)
	expectStatus(t, server.do(http.MethodPost, "/api/motor_modes/abc/like", map[string]int{"like": 1}), http.StatusBadRequest)
}

type riderResponse struct {
	ID                    uint   `json:"id"`
	Login                 string `json:"login"`
	IsBikeServiceEngineer bool   `json:"is_bike_service_engineer"`
}

func TestRegisterRiderHashesPassword(t *testing.T) {
	server := newTestServer()

	recorder := server.do(http.MethodPost, "/api/riders/register", map[string]any{
		"login": "alex_rider", "password": "velo2026", "is_bike_service_engineer": true,
	})
	expectStatus(t, recorder, http.StatusCreated)
	if strings.Contains(recorder.Body.String(), "password") {
		t.Errorf("пароль и хеш не возвращаются: %s", recorder.Body.String())
	}
	rider := decode[riderResponse](t, recorder)
	if rider.ID != 8 || rider.Login != "alex_rider" || rider.IsBikeServiceEngineer {
		t.Errorf("зарегистрированный велосипедист: %+v", rider)
	}
	stored := server.riders.riders["alex_rider"]
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("velo2026")) != nil {
		t.Error("в БД должен лежать bcrypt-хеш пароля")
	}

	expectStatus(t, server.do(http.MethodPost, "/api/riders/register",
		map[string]string{"login": "alex_rider", "password": "другой1"}), http.StatusConflict)
}

func TestRegisterRiderValidatesInput(t *testing.T) {
	server := newTestServer()

	for _, body := range []map[string]string{
		{"login": "ал", "password": "velo2026"},
		{"login": "Alex Rider", "password": "velo2026"},
		{"login": "alex_rider", "password": "12345"},
		{"login": "alex_rider"},
		{"password": "velo2026"},
	} {
		recorder := server.do(http.MethodPost, "/api/riders/register", body)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%v: код %d", body, recorder.Code)
		}
	}
}

func TestLoginStubChecksPassword(t *testing.T) {
	server := newTestServer()
	expectStatus(t, server.do(http.MethodPost, "/api/riders/register",
		map[string]string{"login": "alex_rider", "password": "velo2026"}), http.StatusCreated)

	recorder := server.do(http.MethodPost, "/api/riders/login", map[string]string{"login": "alex_rider", "password": "velo2026"})
	expectStatus(t, recorder, http.StatusOK)
	if !strings.Contains(recorder.Body.String(), "alex_rider") {
		t.Errorf("ответ входа: %s", recorder.Body.String())
	}

	expectStatus(t, server.do(http.MethodPost, "/api/riders/login",
		map[string]string{"login": "alex_rider", "password": "неверный"}), http.StatusUnauthorized)
	expectStatus(t, server.do(http.MethodPost, "/api/riders/login",
		map[string]string{"login": "nobody", "password": "velo2026"}), http.StatusUnauthorized)
	expectStatus(t, server.do(http.MethodPost, "/api/riders/login",
		map[string]string{"login": "alex\x00rider", "password": "velo2026"}), http.StatusBadRequest)
}

func TestLogoutStub(t *testing.T) {
	server := newTestServer()

	recorder := server.do(http.MethodPost, "/api/riders/logout", nil)
	expectStatus(t, recorder, http.StatusOK)
	if decode[messageResponse](t, recorder).Status != "success" {
		t.Errorf("выход: %s", recorder.Body.String())
	}
}

func TestUnknownAPIRouteAnswersJSON(t *testing.T) {
	server := newTestServer()

	recorder := server.do(http.MethodGet, "/api/unknown", nil)
	expectStatus(t, recorder, http.StatusNotFound)
	if decode[messageResponse](t, recorder).Status != "fail" {
		t.Errorf("неизвестный метод API: %s", recorder.Body.String())
	}

	recorder = server.do(http.MethodGet, "/motor_modes/unknown", nil)
	if recorder.Body.String() != "ssr" {
		t.Errorf("страницы вне /api отдаёт SSR: %s", recorder.Body.String())
	}
}

func TestWrongMethodAnswersJSON405(t *testing.T) {
	server := newTestServer()

	recorder := server.do(http.MethodPut, "/api/motor_modes/1", nil)
	expectStatus(t, recorder, http.StatusMethodNotAllowed)
	if decode[messageResponse](t, recorder).Status != "fail" {
		t.Errorf("неверный метод: %s", recorder.Body.String())
	}

	recorder = server.do(http.MethodPost, "/api/motor_modes/feed", nil)
	expectStatus(t, recorder, http.StatusMethodNotAllowed)
}

func TestEmptyFilterMeansAllModes(t *testing.T) {
	server := newTestServer()

	list := decode[listResponse](t, server.do(http.MethodGet, "/api/motor_modes?max_consumption_wh_per_km=", nil))
	if len(list.MotorModes) != 2 {
		t.Errorf("пустой фильтр должен показывать все опубликованные режимы: %+v", list)
	}
}
