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

// Package apcu 提供了进程内缓存组件。
package apcu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/go-spring/spring-base/fastdev"
	"github.com/go-spring/spring-base/fastdev/recorder"
	"github.com/go-spring/spring-base/fastdev/replayer"
)

var cache sync.Map

// EmptyValue 流量录制时表示空值。
const EmptyValue = "::empty::"

func getKey(ctx context.Context, key string) (string, error) {
	if replayer.ReplayMode() {
		sessionID, err := replayer.GetSessionID(ctx)
		if err != nil {
			return "", err
		}
		return sessionID + key, nil
	}
	return key, nil
}

// Load 获取 key 对应的缓存值，注意 out 的类型必须和 Store 的时候存入的类
// 型一致，否则 Load 会失败。但是如果 Store 的时候存入的内容是一个字符串，
// 那么 out 可以是该字符串 JSON 反序列化之后的类型。
func Load(ctx context.Context, key string, out interface{}) (ok bool, err error) {

	defer func() {
		if err == nil && recorder.RecordMode() {
			resp := interface{}(EmptyValue)
			if ok {
				resp = out
			}
			recorder.RecordAction(ctx, &recorder.Action{
				Protocol: fastdev.APCU,
				Request:  recorder.NewMessage(key),
				Response: recorder.NewMessage(resp),
			})
		}
	}()

	if key, err = getKey(ctx, key); err != nil {
		return false, err
	}
	return load(key, out)
}

type cacheItem struct {
	source   interface{}
	expireAt time.Time
}

func load(key string, out interface{}) (ok bool, err error) {

	v, ok := cache.Load(key)
	if !ok {
		return false, nil
	}

	item := v.(*cacheItem)
	if item.source == nil {
		return false, nil
	}

	// 缓存过期之后不会删除 key 对应的 *cacheItem 对象，而是将 *cacheItem
	// 对象的 source 设为 nil，原因是此时此处的 Delete 操作无法和此时别处的
	// Store 操作保持顺序，即此处检测到了过期，但是别处正在执行 Store 操作，
	// 那么此处的 Delete 理应在 Store 之前执行，但是目前的框架下是无法保证的。
	// 因此，退而求其次，把缓存的真实内容释放掉，这样即使占了一些内存也不会太多。
	if !item.expireAt.IsZero() && time.Now().After(item.expireAt) {
		cache.Store(key, &cacheItem{expireAt: item.expireAt})
		return false, nil
	}

	outVal := reflect.ValueOf(out)
	if outVal.Kind() != reflect.Ptr || outVal.IsNil() {
		return false, errors.New("out value should be ptr and not nil")
	}

	switch source := item.source.(type) {
	case string:
		outType := reflect.TypeOf(out)
		val := reflect.New(outType.Elem())
		err = json.Unmarshal([]byte(source), val.Interface())
		if err != nil {
			if outVal.Elem().Kind() == reflect.String {
				outVal.Elem().SetString(source)
				return true, nil
			}
			return false, err
		}
		item.source = val.Elem()
		outVal.Elem().Set(val.Elem())
		return true, nil
	case reflect.Value:
		if outVal.Type().Elem() == source.Type() {
			outVal.Elem().Set(source)
			return true, nil
		}
	default:
		srcVal := reflect.ValueOf(source)
		if srcVal.Type() == outVal.Type().Elem() {
			outVal.Elem().Set(srcVal)
			return true, nil
		}
	}
	return false, fmt.Errorf("type not match %s", outVal.Elem().Type())
}

type StoreArg struct {
	TTL time.Duration
}

type StoreOption func(arg *StoreArg)

// TTL 设置 key 的过期时间。
func TTL(ttl time.Duration) StoreOption {
	return func(arg *StoreArg) {
		arg.TTL = ttl
	}
}

// Store 保存 key 及其对应的 val，支持对 key 设置 ttl (过期时间)。另外，
// 这里的 val 可以是任何值，因此要求 Load 的时候必须保证返回值和这里的 val
// 是相同类型的，否则 Load 会失败。
// 但是这里有一个例外情况，考虑到很多场景下，用户需要缓存一个由字符串反序列
// 化后的对象，所以该库提供了一个功能，就是用户可以 Store 一个字符串，然后
// Load 的时候按照指定类型返回。
func Store(ctx context.Context, key string, val interface{}, opts ...StoreOption) error {
	arg := StoreArg{}
	for _, opt := range opts {
		opt(&arg)
	}
	expireAt := time.Time{}
	if arg.TTL > 0 {
		expireAt = time.Now().Add(arg.TTL)
	}
	key, err := getKey(ctx, key)
	if err != nil {
		return err
	}
	cache.Store(key, &cacheItem{source: val, expireAt: expireAt})
	return nil
}

// Delete 删除 key 对应的缓存内容。
func Delete(ctx context.Context, key string) {
	key, _ = getKey(ctx, key)
	cache.Delete(key)
}

// Range 遍历缓存的内容。
func Range(f func(key, value interface{}) bool) {
	cache.Range(func(key, value interface{}) bool {
		return f(key, value.(*cacheItem).source)
	})
}
