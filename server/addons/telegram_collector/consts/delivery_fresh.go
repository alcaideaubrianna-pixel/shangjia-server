package consts

// QueueDeliveryFresh only accepts the first queued realtime Event handoff.
// It must never be selected by replay or Delivery recovery.
const QueueDeliveryFresh = "tg.delivery.fresh.v1"
