package model

type MediaSearchScope struct {
	AccountIds []int64
	Partitions []MediaSearchScopePartition
}

type MediaSearchScopePartition struct {
	TenantId   int64
	AccountIds []int64
}
