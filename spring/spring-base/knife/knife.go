/*
 * Copyright 2012-2019 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package knife 提供了 context.Context 上的缓存。
package knife

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type ctxKeyType int

var ctxKey ctxKeyType

func cache(ctx context.Context) (*sync.Map, bool) {
	m, ok := ctx.Value(ctxKey).(*sync.Map)
	return m, ok
}

// New 返回带有缓存空间的 context.Context 对象，已绑定缓存空间时 cached 返回 true 。
func New(ctx context.Context) (c context.Context, cached bool) {
	if _, ok := cache(ctx); ok {
		return ctx, true
	}
	c = context.WithValue(ctx, ctxKey, new(sync.Map))
	return c, false
}

// Copy 拷贝 context.Context 对象中的内容到另一个 context.Context 对象。
func Copy(src context.Context, keys ...string) (context.Context, error) {
	m, ok := cache(src)
	if !ok {
		return nil, nil
	}
	dest, _ := New(context.Background())
	if len(keys) == 0 {
		c, _ := cache(dest)
		m.Range(func(key, value interface{}) bool {
			c.Store(key, value)
			return true
		})
	} else {
		for _, key := range keys {
			var v interface{}
			if v, ok = m.Load(key); ok {
				if err := Set(dest, key, v); err != nil {
					return nil, err
				}
			}
		}
	}
	return dest, nil
}

// Get 从 context.Context 对象中获取 key 对应的 val。
func Get(ctx context.Context, key string) (interface{}, bool) {
	if m, ok := cache(ctx); ok {
		return m.Load(key)
	}
	return nil, false
}

// Set 将 key 及其 val 保存到 context.Context 对象。
func Set(ctx context.Context, key string, val interface{}) error {
	m, ok := cache(ctx)
	if !ok {
		return errors.New("knife uninitialized")
	}
	if _, loaded := m.LoadOrStore(key, val); loaded {
		return fmt.Errorf("duplicate key %s", key)
	}
	return nil
}

// Delete 从 context.Context 对象中删除 key 及其对应的 val 。
func Delete(ctx context.Context, key string) {
	if m, ok := cache(ctx); ok {
		m.Delete(key)
	}
}

// Range 遍历 context.Context 对象中所有的 key 和 val 。
func Range(ctx context.Context, f func(key, value interface{}) bool) {
	if m, ok := cache(ctx); ok {
		m.Range(f)
	}
}
