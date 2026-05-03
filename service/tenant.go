package service

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type tenantCacheEntry struct {
	tenant    *model.Tenant // nil means "not found"
	expiresAt time.Time
}

var (
	tenantDomainCache   = map[string]tenantCacheEntry{}
	tenantDomainCacheMu sync.RWMutex
	tenantCacheTTL      = 60 * time.Second
)

func ResolveTenantByHost(host string) (*model.Tenant, error) {
	host = strings.ToLower(strings.Split(host, ":")[0])
	if host == "" {
		return nil, nil
	}

	tenantDomainCacheMu.RLock()
	entry, ok := tenantDomainCache[host]
	tenantDomainCacheMu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.tenant, nil
	}

	return lookupTenantByDomain(host)
}

func lookupTenantByDomain(host string) (*model.Tenant, error) {
	var domain model.TenantDomain
	err := model.DB.Where("domain = ? AND status = 1", host).First(&domain).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// negative cache: host not bound to any tenant
		tenantDomainCacheMu.Lock()
		tenantDomainCache[host] = tenantCacheEntry{nil, time.Now().Add(tenantCacheTTL)}
		tenantDomainCacheMu.Unlock()
		return nil, nil
	}
	if err != nil {
		return nil, err // real DB error: caller should fail-closed
	}

	var tenant model.Tenant
	if err := model.DB.First(&tenant, domain.TenantId).Error; err != nil {
		return nil, err
	}

	tenantDomainCacheMu.Lock()
	tenantDomainCache[host] = tenantCacheEntry{&tenant, time.Now().Add(tenantCacheTTL)}
	tenantDomainCacheMu.Unlock()
	return &tenant, nil
}

func ResolveTenantByCode(code string) (*model.Tenant, error) {
	if code == "" {
		return nil, nil
	}
	var tenant model.Tenant
	if err := model.DB.Where("code = ?", code).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tenant, nil
}

func InvalidateTenantDomainCache() {
	tenantDomainCacheMu.Lock()
	tenantDomainCache = map[string]tenantCacheEntry{}
	tenantDomainCacheMu.Unlock()
}
