package service

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/go-telegram/bot/models"
)

type BotContext struct {
	Key      string
	Token    string
	Bindings []BotBinding
}

type MenuItem struct {
	Command     string
	Description string
	Order       int
}

type FeatureMenus struct {
	Managed bool
	Items   []MenuItem
}

type Feature interface {
	Key() string
	Priority() int
	Menus(ctx context.Context, bot BotContext) (FeatureMenus, error)
	HandleUpdate(ctx context.Context, bot BotContext, update *models.Update) (bool, error)
}

var (
	featuresMu sync.RWMutex
	features   = map[string]Feature{}
)

func RegisterFeature(feature Feature) {
	if feature == nil || strings.TrimSpace(feature.Key()) == "" {
		return
	}
	featuresMu.Lock()
	features[feature.Key()] = feature
	featuresMu.Unlock()
}

func Features() []Feature {
	featuresMu.RLock()
	items := make([]Feature, 0, len(features))
	for _, feature := range features {
		items = append(items, feature)
	}
	featuresMu.RUnlock()
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority() == items[j].Priority() {
			return items[i].Key() < items[j].Key()
		}
		return items[i].Priority() > items[j].Priority()
	})
	return items
}
