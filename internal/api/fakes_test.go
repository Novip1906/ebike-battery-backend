package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"ebike-battery-backend/internal/ds"
	"ebike-battery-backend/internal/repository"
)

type likeKey struct {
	riderID     uint
	motorModeID uint
}

type fakeMotorModes struct {
	modes  map[uint]*ds.MotorMode
	likes  map[likeKey]bool
	nextID uint
}

func ptr[T any](value T) *T {
	return &value
}

func newFakeMotorModes() *fakeMotorModes {
	created := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	published := created.Add(24 * time.Hour)
	repo := &fakeMotorModes{
		modes: map[uint]*ds.MotorMode{
			1: {ID: 1, ModeName: "Eco", ShortDescription: ptr("Слабая поддержка"), Status: ds.StatusPublished,
				ImageKey: "eco_mode.jpg", VideoKey: "eco_mode.mp4", SupportPercent: ptr(60),
				ConsumptionWhPerKm: ptr(5.2), CreatedAt: created, CreatorRiderID: 7, PublishedAt: &published},
			2: {ID: 2, ModeName: "Race", ShortDescription: ptr("Гоночный режим"), Status: ds.StatusPublished,
				ImageKey: "race_mode.jpg", VideoKey: "race_mode.mp4", SupportPercent: ptr(400),
				ConsumptionWhPerKm: ptr(18.6), CreatedAt: created, CreatorRiderID: 1, PublishedAt: &published},
			3: {ID: 3, ModeName: "Limit", Status: ds.StatusDraft, CreatedAt: created, CreatorRiderID: 2},
			4: {ID: 4, ModeName: "Off", Status: ds.StatusDeleted, SupportPercent: ptr(0),
				ConsumptionWhPerKm: ptr(0.0), CreatedAt: created, CreatorRiderID: 1},
		},
		likes: map[likeKey]bool{
			{riderID: 2, motorModeID: 1}: true,
			{riderID: 3, motorModeID: 1}: true,
			{riderID: 1, motorModeID: 2}: true,
		},
		nextID: 5,
	}
	return repo
}

func (f *fakeMotorModes) sortedIDs() []uint {
	ids := make([]uint, 0, len(f.modes))
	for id := range f.modes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (f *fakeMotorModes) PublishedModes(maxConsumption float64) ([]ds.MotorMode, error) {
	var result []ds.MotorMode
	for _, id := range f.sortedIDs() {
		mode := f.modes[id]
		if mode.Status == ds.StatusPublished && mode.ConsumptionWhPerKm != nil && *mode.ConsumptionWhPerKm <= maxConsumption {
			result = append(result, *mode)
		}
	}
	return result, nil
}

func (f *fakeMotorModes) ConsumptionBounds() (float64, float64, error) {
	minValue, maxValue := math.Inf(1), 0.0
	for _, mode := range f.modes {
		if mode.Status == ds.StatusPublished && mode.ConsumptionWhPerKm != nil && *mode.ConsumptionWhPerKm > 0 {
			minValue = math.Min(minValue, *mode.ConsumptionWhPerKm)
			maxValue = math.Max(maxValue, *mode.ConsumptionWhPerKm)
		}
	}
	if maxValue == 0 {
		return 0, 0, nil
	}
	return minValue, maxValue, nil
}

func (f *fakeMotorModes) LikeCounts() (map[uint]int, error) {
	counts := map[uint]int{}
	for key := range f.likes {
		counts[key.motorModeID]++
	}
	return counts, nil
}

func (f *fakeMotorModes) LikeCount(motorModeID uint) (int, error) {
	counts, _ := f.LikeCounts()
	return counts[motorModeID], nil
}

func (f *fakeMotorModes) IsLikedBy(riderID, motorModeID uint) (bool, error) {
	return f.likes[likeKey{riderID: riderID, motorModeID: motorModeID}], nil
}

func (f *fakeMotorModes) FirstPublishedMode() (ds.MotorMode, error) {
	for _, id := range f.sortedIDs() {
		if f.modes[id].Status == ds.StatusPublished {
			return *f.modes[id], nil
		}
	}
	return ds.MotorMode{}, repository.ErrMotorModeNotFound
}

func (f *fakeMotorModes) NextPublishedMode(afterID uint) (ds.MotorMode, error) {
	for _, id := range f.sortedIDs() {
		if id > afterID && f.modes[id].Status == ds.StatusPublished {
			return *f.modes[id], nil
		}
	}
	return f.FirstPublishedMode()
}

func (f *fakeMotorModes) PublishedModeByID(id uint) (ds.MotorMode, error) {
	mode, ok := f.modes[id]
	if !ok || mode.Status != ds.StatusPublished {
		return ds.MotorMode{}, repository.ErrMotorModeNotFound
	}
	return *mode, nil
}

func (f *fakeMotorModes) ActiveModeByID(id uint) (ds.MotorMode, error) {
	mode, ok := f.modes[id]
	if !ok || mode.Status == ds.StatusDeleted {
		return ds.MotorMode{}, repository.ErrMotorModeNotFound
	}
	return *mode, nil
}

func (f *fakeMotorModes) DraftByRider(riderID uint) (ds.MotorMode, error) {
	for _, id := range f.sortedIDs() {
		mode := f.modes[id]
		if mode.Status == ds.StatusDraft && mode.CreatorRiderID == riderID {
			return *mode, nil
		}
	}
	return ds.MotorMode{}, repository.ErrMotorModeNotFound
}

func (f *fakeMotorModes) CreateDraftWithMedia(riderID uint, modeName string, attachMedia func(uint) (repository.MediaKeys, error)) (ds.MotorMode, error) {
	if _, err := f.DraftByRider(riderID); err == nil {
		return ds.MotorMode{}, repository.ErrDraftAlreadyExists
	}
	mode := ds.MotorMode{ID: f.nextID, ModeName: modeName, Status: ds.StatusDraft, CreatedAt: time.Now(), CreatorRiderID: riderID}
	f.nextID++
	keys, err := attachMedia(mode.ID)
	if err != nil {
		return ds.MotorMode{}, err
	}
	mode.ImageKey, mode.VideoKey = keys.ImageKey, keys.VideoKey
	f.modes[mode.ID] = &mode
	return mode, nil
}

func (f *fakeMotorModes) PublishDraft(riderID, draftID uint, fields repository.PublishFields) error {
	mode, ok := f.modes[draftID]
	if !ok || mode.Status != ds.StatusDraft || mode.CreatorRiderID != riderID {
		return repository.ErrMotorModeNotFound
	}
	now := time.Now()
	mode.ModeName = fields.ModeName
	mode.ShortDescription = &fields.ShortDescription
	mode.SupportPercent = &fields.SupportPercent
	mode.ConsumptionWhPerKm = &fields.ConsumptionWhPerKm
	mode.Status = ds.StatusPublished
	mode.PublishedAt = &now
	return nil
}

func (f *fakeMotorModes) MarkDeleted(riderID, motorModeID uint) error {
	mode, ok := f.modes[motorModeID]
	if !ok || mode.CreatorRiderID != riderID || mode.Status == ds.StatusDeleted {
		return repository.ErrMotorModeNotFound
	}
	mode.Status = ds.StatusDeleted
	return nil
}

func (f *fakeMotorModes) SetLike(riderID, motorModeID uint, liked bool) error {
	key := likeKey{riderID: riderID, motorModeID: motorModeID}
	if liked {
		f.likes[key] = true
	} else {
		delete(f.likes, key)
	}
	return nil
}

type storedObject struct {
	contentType string
	content     []byte
}

type fakeStorage struct {
	objects      map[string]storedObject
	failPut      bool
	failOnSuffix string
	afterPut     func(key string)
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: map[string]storedObject{}}
}

func (s *fakeStorage) Put(ctx context.Context, key string, content io.Reader, _ int64, contentType string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.failPut || (s.failOnSuffix != "" && strings.HasSuffix(key, s.failOnSuffix)) {
		return errors.New("minio недоступен")
	}
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, content); err != nil {
		return err
	}
	s.objects[key] = storedObject{contentType: contentType, content: buffer.Bytes()}
	if s.afterPut != nil {
		s.afterPut(key)
	}
	return nil
}

func (s *fakeStorage) Remove(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delete(s.objects, key)
	return nil
}

type fakeRiders struct {
	riders map[string]ds.Rider
	nextID uint
}

func newFakeRiders() *fakeRiders {
	return &fakeRiders{riders: map[string]ds.Rider{}, nextID: 8}
}

func (f *fakeRiders) CreateRider(login, passwordHash string) (ds.Rider, error) {
	if _, ok := f.riders[login]; ok {
		return ds.Rider{}, repository.ErrLoginTaken
	}
	rider := ds.Rider{ID: f.nextID, Login: login, PasswordHash: passwordHash}
	f.nextID++
	f.riders[login] = rider
	return rider, nil
}

func (f *fakeRiders) RiderByLogin(login string) (ds.Rider, error) {
	rider, ok := f.riders[login]
	if !ok {
		return ds.Rider{}, repository.ErrRiderNotFound
	}
	return rider, nil
}

type blacklistEntry struct {
	riderLogin string
	ttl        time.Duration
}

type fakeBlacklist struct {
	entries map[string]blacklistEntry
	fail    bool
}

func newFakeBlacklist() *fakeBlacklist {
	return &fakeBlacklist{entries: map[string]blacklistEntry{}}
}

func (b *fakeBlacklist) Add(_ context.Context, token, riderLogin string, ttl time.Duration) error {
	if b.fail {
		return errors.New("redis недоступен")
	}
	b.entries[token] = blacklistEntry{riderLogin: riderLogin, ttl: ttl}
	return nil
}

func (b *fakeBlacklist) Contains(_ context.Context, token string) (bool, error) {
	if b.fail {
		return false, errors.New("redis недоступен")
	}
	_, ok := b.entries[token]
	return ok, nil
}
