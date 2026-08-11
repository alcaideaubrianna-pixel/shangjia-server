package consts

const (
	MediaCachePrefix        = "tg:collector:media:"
	MediaLockPrefix         = "tg:collector:media-lock:"
	AccountOwnerPrefix      = "youban_publish:tg:account-client:"
	AccountLeaseEpochPrefix = "tg:collector:account-lease-epoch:"
	MediaCacheDefaultHours  = 720
)

func MediaCacheKey(fingerprint string) string {
	return MediaCachePrefix + fingerprint
}

func MediaLockKey(fingerprint string) string {
	return MediaLockPrefix + fingerprint
}

func AccountOwnerKey(accountID int64) string {
	return AccountOwnerPrefix + formatInt64(accountID)
}

func AccountLeaseEpochKey(accountID int64) string {
	return AccountLeaseEpochPrefix + formatInt64(accountID)
}

func formatInt64(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := [20]byte{}
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buf[index] = '-'
	}
	return string(buf[index:])
}
