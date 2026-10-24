package sys

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

func (s *sGateway) dispatch(ctx context.Context, key string, update *models.Update) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dispatchStartedAt := time.Now()
	if update != nil && update.InlineQuery != nil {
		g.Log().Infof(ctx, "TG链路 gateway_inline_dispatch_start key:%s updateId:%d queryId:%s", key, update.ID, update.InlineQuery.ID)
		defer func() {
			g.Log().Infof(ctx, "TG链路 gateway_inline_dispatch_end key:%s updateId:%d queryId:%s duration:%s", key, update.ID, update.InlineQuery.ID, time.Since(dispatchStartedAt))
		}()
	}
	if update != nil && update.CallbackQuery != nil {
		query := update.CallbackQuery
		g.Log().Infof(ctx, "TG链路 gateway_callback_dispatch_start key:%s updateId:%d callbackId:%s userId:%d data:%s", key, update.ID, query.ID, query.From.ID, strings.TrimSpace(query.Data))
		defer func() {
			g.Log().Infof(ctx, "TG链路 gateway_callback_dispatch_end key:%s updateId:%d callbackId:%s duration:%s", key, update.ID, query.ID, time.Since(dispatchStartedAt))
		}()
	}
	s.mu.Lock()
	bindings := append([]service.BotBinding(nil), s.bindings[key]...)
	s.mu.Unlock()
	if len(bindings) == 0 {
		loaded, err := s.loadBindings(ctx)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.bindings = loaded
		bindings = append(bindings, s.bindings[key]...)
		s.mu.Unlock()
	}
	if update != nil && update.InlineQuery != nil {
		for _, binding := range bindings {
			g.Log().Infof(ctx, "TG链路 gateway_inline_binding key:%s updateId:%d owner:%s reference:%d tenant:%d", key, update.ID, binding.Owner, binding.ReferenceID, binding.TenantID)
		}
		if len(bindings) == 0 {
			g.Log().Warningf(ctx, "TG链路 gateway_inline_no_binding key:%s updateId:%d queryId:%s", key, update.ID, update.InlineQuery.ID)
		}
	}
	if update != nil && update.CallbackQuery != nil && len(bindings) == 0 {
		g.Log().Warningf(ctx, "TG链路 gateway_callback_no_binding key:%s updateId:%d callbackId:%s userId:%d data:%s", key, update.ID, update.CallbackQuery.ID, update.CallbackQuery.From.ID, strings.TrimSpace(update.CallbackQuery.Data))
	}
	logGatewayUpdate(ctx, update, len(bindings))
	botCtx := service.BotContext{Key: key, Bindings: bindings}
	if len(bindings) > 0 {
		botCtx.Token = bindings[0].Token
	}
	var featureErrors []error
	for _, feature := range service.Features() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if update != nil && update.InlineQuery != nil {
			g.Log().Infof(ctx, "TG链路 gateway_inline_feature_start updateId:%d feature:%s", update.ID, feature.Key())
		}
		handled, featureErr := feature.HandleUpdate(ctx, botCtx, update)
		if update != nil && update.InlineQuery != nil {
			g.Log().Infof(ctx, "TG链路 gateway_inline_feature_end updateId:%d feature:%s handled:%t err:%v", update.ID, feature.Key(), handled, featureErr)
		}
		if featureErr != nil {
			if handled {
				return gerror.Wrapf(featureErr, "TG Bot功能已接管消息但处理失败 feature:%s", feature.Key())
			}
			featureErrors = append(featureErrors, gerror.Wrapf(featureErr, "TG Bot附加功能处理失败 feature:%s", feature.Key()))
			continue
		}
		if handled {
			g.Log().Infof(ctx, "TG Bot Gateway更新已由功能消费 update:%d feature:%s", update.ID, feature.Key())
			counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.gateway_updates_dispatched")
			counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "handled")))
			return nil
		}
	}
	providers := map[string]service.Provider{}
	for _, provider := range service.Providers() {
		providers[provider.Name()] = provider
	}
	for _, binding := range bindings {
		provider := providers[binding.Owner]
		if err := ctx.Err(); err != nil {
			return err
		}
		if provider == nil {
			continue
		}
		if update != nil && update.InlineQuery != nil {
			g.Log().Infof(ctx, "TG链路 gateway_inline_provider_start updateId:%d provider:%s reference:%d", update.ID, binding.Owner, binding.ReferenceID)
		}
		if err := provider.HandleUpdate(ctx, binding, update); err != nil {
			if update != nil && update.InlineQuery != nil {
				g.Log().Warningf(ctx, "TG链路 gateway_inline_provider_error updateId:%d provider:%s reference:%d err:%+v", update.ID, binding.Owner, binding.ReferenceID, err)
			}
			return err
		}
		if update != nil && update.InlineQuery != nil {
			g.Log().Infof(ctx, "TG链路 gateway_inline_provider_end updateId:%d provider:%s reference:%d", update.ID, binding.Owner, binding.ReferenceID)
		}
		g.Log().Infof(ctx, "TG Bot Gateway更新已分发 update:%d provider:%s reference:%d", update.ID, binding.Owner, binding.ReferenceID)
	}
	if len(featureErrors) > 0 {
		return errors.Join(featureErrors...)
	}
	counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.gateway_updates_dispatched")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "provider_dispatch")))
	return nil
}
