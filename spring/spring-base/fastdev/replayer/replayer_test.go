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

package replayer_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-spring/spring-base/assert"
	"github.com/go-spring/spring-base/cast"
	"github.com/go-spring/spring-base/chrono"
	"github.com/go-spring/spring-base/fastdev"
	"github.com/go-spring/spring-base/fastdev/replayer"
	"github.com/go-spring/spring-base/knife"
)

func init() {
	fastdev.RegisterProtocol(fastdev.HTTP, &httpProtocol{})
	fastdev.RegisterProtocol(fastdev.REDIS, &redisProtocol{})
}

func TestReplayAction(t *testing.T) {

	replayer.SetReplayMode(true)
	defer func() {
		replayer.SetReplayMode(false)
	}()

	sessionID := "39fc5c13443f47da9ff320cc4b02c789"
	ctx, _ := knife.New(context.Background())
	err := replayer.SetSessionID(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	recordSession := &fastdev.Session{
		Session:   sessionID,
		Timestamp: chrono.Now(ctx).UnixNano(),
		Inbound: &fastdev.Action{
			Protocol:  fastdev.HTTP,
			Timestamp: chrono.Now(ctx).UnixNano(),
			Request: fastdev.NewMessage(func() string {
				return "GET ..."
			}),
			Response: fastdev.NewMessage(func() string {
				return "200 ..."
			}),
		},
		Actions: []*fastdev.Action{
			{
				Protocol:  fastdev.REDIS,
				Timestamp: chrono.Now(ctx).UnixNano(),
				Request: fastdev.NewMessage(func() string {
					return cast.ToCommandLine("SET", "a", "1")
				}),
				Response: fastdev.NewMessage(func() string {
					return cast.ToCSV(1, "2", 3)
				}),
			}, {
				Protocol:  fastdev.REDIS,
				Timestamp: chrono.Now(ctx).UnixNano(),
				Request: fastdev.NewMessage(func() string {
					return cast.ToCommandLine("SET", "a", "\x00\xc0\n\t\x00\xbem\x06\x89Z(\x00\n")
				}),
				Response: fastdev.NewMessage(func() string {
					return cast.ToCSV("\x00\xc0\n\t\x00\xbem\x06\x89Z(\x00\n")
				}),
			},
			{
				Protocol:  fastdev.REDIS,
				Timestamp: chrono.Now(ctx).UnixNano(),
				Request: fastdev.NewMessage(func() string {
					return cast.ToCommandLine("HGET", "a")
				}),
				Response: fastdev.NewMessage(func() string {
					return cast.ToCSV("a", "b", "c", 3, "d", "\x00\xc0\n\t\x00\xbem\x06\x89Z(\x00\n")
				}),
			},
		},
	}

	str, err := recordSession.Pretty()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("record:", str)

	rawSession, err := fastdev.ToRawSession(str)
	if err != nil {
		t.Fatal(err)
	}

	session, err := replayer.ToSession(rawSession)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Print("json(record): ")
	fmt.Println(session.Pretty())

	err = replayer.Store(session)
	if err != nil {
		t.Fatal(err)
	}

	{
		var (
			action  *replayer.Action
			request = cast.ToCommandLine("SET", "a", "1")
		)
		action, err = replayer.ReplayAction(ctx, fastdev.REDIS, request)
		assert.Nil(t, err)
		assert.NotNil(t, action)

		fmt.Print("action: ")
		fmt.Println(action.Pretty())
	}

	{
		var (
			action  *replayer.Action
			request = cast.ToCommandLine("SET", "a", "\x00\xc0\n\t\x00\xbem\x06\x89Z(\x00\n")
		)
		action, err = replayer.ReplayAction(ctx, fastdev.REDIS, request)
		assert.Nil(t, err)
		assert.NotNil(t, action)

		fmt.Print("action: ")
		fmt.Println(action.Pretty())
	}

	err = replayer.ReplayInbound(ctx, "200 ...")
	if err != nil {
		t.Fatal(err)
	}

	err = session.Flat()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(session.Pretty())
}

type httpProtocol struct{}

func (p *httpProtocol) ShouldDiff() bool {
	return true
}

func (p *httpProtocol) GetLabel(data string) string {
	return data[:4]
}

func (p *httpProtocol) FlatRequest(data string) (map[string]string, error) {
	return nil, nil
}

func (p *httpProtocol) FlatResponse(data string) (map[string]string, error) {
	return nil, nil
}

type redisProtocol struct{}

func (p *redisProtocol) ShouldDiff() bool {
	return true
}

func (p *redisProtocol) GetLabel(data string) string {
	return data[:4]
}

func (p *redisProtocol) FlatRequest(data string) (map[string]string, error) {
	csv, err := cast.ParseCommandLine(data)
	if err != nil {
		return nil, err
	}
	return cast.FlatSlice(csv), nil
}

func (p *redisProtocol) FlatResponse(data string) (map[string]string, error) {
	csv, err := cast.ParseCSV(data)
	if err != nil {
		return nil, err
	}
	return cast.FlatSlice(csv), nil
}
