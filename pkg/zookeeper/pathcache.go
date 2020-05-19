/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package zookeeper

import (
	"github.com/samuel/go-zookeeper/zk"
	"istio.io/istio/pkg/log"
	"path"
)

type pathCacheEventType int

type pathCache struct {
	conn       *zk.Conn
	watchCh    chan zk.Event
	notifyCh   chan pathCacheEvent
	stopCh     chan bool
	addChildCh chan string
	path       string
	cached     map[string]bool
}

type pathCacheEvent struct {
	eventType pathCacheEventType
	path      string
}

const (
	pathCacheEventAdded pathCacheEventType = iota
	pathCacheEventDeleted
)

func newPathCache(conn *zk.Conn, path string) (*pathCache, error) {
	p := &pathCache{
		conn:   conn,
		path:   path,
		cached: make(map[string]bool),

		watchCh:    make(chan zk.Event),
		notifyCh:   make(chan pathCacheEvent),
		addChildCh: make(chan string),
		stopCh:     make(chan bool),
	}

	err := p.watchChildren()
	if err != nil {
		log.Warnf("Failed to watch zk path %s, %s", path, err)
		return nil, err
	}

	go func() {
		for {
			select {
			case child := <-p.addChildCh:
				p.onChildAdd(child)
			case event := <-p.watchCh:
				p.onEvent(&event)
			case <-p.stopCh:
				close(p.notifyCh)
				return
			}
		}
	}()

	return p, nil
}

func (p *pathCache) events() <-chan pathCacheEvent {
	return p.notifyCh
}

func (p *pathCache) stop() {
	go func() {
		p.stopCh <- true
	}()
}

func (p *pathCache) watch(path string) error {
	_, _, ch, err := p.conn.GetW(path)
	if err != nil {
		return err
	}
	go p.forward(ch)
	return nil
}

func (p *pathCache) watchChildren() error {
	children, _, ch, err := p.conn.ChildrenW(p.path)
	if err != nil {
		return err
	}
	go p.forward(ch)
	for _, child := range children {
		fp := path.Join(p.path, child)
		if ok := p.cached[fp]; !ok {
			go p.addChild(fp)
		}
	}
	return nil
}

func (p *pathCache) onChildAdd(child string) {
	err := p.watch(child)
	if err != nil {
		log.Warnf("Failed to watch child %s, the err is %s", child, err)
		return
	}
	p.cached[child] = true
	event := pathCacheEvent{
		eventType: pathCacheEventAdded,
		path:      child,
	}
	go p.notify(event)
}

func (p *pathCache) onEvent(event *zk.Event) {
	switch event.Type {
	case zk.EventNodeChildrenChanged:
		p.watchChildren()
	case zk.EventNodeDeleted:
		p.onChildDeleted(event.Path)
	}
}

func (p *pathCache) onChildDeleted(child string) {
	vent := pathCacheEvent{
		eventType: pathCacheEventDeleted,
		path:      child,
	}
	go p.notify(vent)
}

func (p *pathCache) addChild(child string) {
	p.addChildCh <- child
}

func (p *pathCache) notify(event pathCacheEvent) {
	p.notifyCh <- event
}

func (p *pathCache) forward(eventCh <-chan zk.Event) {
	event, ok := <-eventCh
	if ok {
		p.watchCh <- event
	}
}
