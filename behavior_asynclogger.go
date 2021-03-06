// Copyright (c) 2012 - Cloud Instruments Co., Ltd.
// 
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met: 
// 
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer. 
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution. 
// 
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package seelog

import (
	"container/list"
	"sync"
	"fmt"
	"errors"
)

// MaxQueueSize is the critical number of messages in the queue that result in an immediate flush.
const (
	MaxQueueSize = 10000
)

type msgQueueItem struct {
	level   LogLevel
	context logContextInterface
	message  fmt.Stringer
}

// asyncLogger represents common data for all asynchronous loggers
type asyncLogger struct {
	commonLogger
	msgQueue         *list.List
	queueMutex       *sync.Mutex
	queueHasElements *sync.Cond
}

// newAsyncLogger creates a new asynchronous logger
func newAsyncLogger(config *logConfig) *asyncLogger {
	asnLogger := new(asyncLogger)

	asnLogger.msgQueue = list.New()
	asnLogger.queueMutex = new(sync.Mutex)
	asnLogger.queueHasElements = sync.NewCond(new(sync.Mutex))

	asnLogger.commonLogger = *newCommonLogger(config, asnLogger)

	return asnLogger
}

func (asnLogger *asyncLogger) innerLog(
	level LogLevel,
	context logContextInterface,
	message fmt.Stringer) {

	asnLogger.addMsgToQueue(level, context, message)
}

func (asnLogger *asyncLogger) Close() {
	asnLogger.queueMutex.Lock()
	defer asnLogger.queueMutex.Unlock()

	if !asnLogger.closed {
		asnLogger.flushQueue()
		asnLogger.config.RootDispatcher.Flush()
		asnLogger.config.RootDispatcher.Close()
		asnLogger.queueHasElements.Broadcast()
	}
}

func (asnLogger *asyncLogger) Flush() {
	asnLogger.queueMutex.Lock()
	defer asnLogger.queueMutex.Unlock()

	if !asnLogger.closed {
		asnLogger.flushQueue()
		asnLogger.config.RootDispatcher.Flush()
	}
}

func (asnLogger *asyncLogger) flushQueue() {
	asnLogger.queueHasElements.L.Lock()
	defer asnLogger.queueHasElements.L.Unlock()

	for asnLogger.msgQueue.Len() > 0 {
		asnLogger.processQueueElement()
	}
}

func (asnLogger *asyncLogger) processQueueElement() {
	if asnLogger.msgQueue.Len() > 0 {
		backElement := asnLogger.msgQueue.Front()
		msg, _ := backElement.Value.(msgQueueItem)
		asnLogger.processLogMsg(msg.level, msg.message, msg.context)
		asnLogger.msgQueue.Remove(backElement)
	}
}

func (asnLogger *asyncLogger) addMsgToQueue(
	level LogLevel,
	context logContextInterface,
	message fmt.Stringer) {
	asnLogger.queueMutex.Lock()
	defer asnLogger.queueMutex.Unlock()

	if !asnLogger.closed {
		if asnLogger.msgQueue.Len() >= MaxQueueSize {
			fmt.Printf("Seelog queue overflow: more than %v messages in the queue. Flushing.\n", MaxQueueSize)
			asnLogger.flushQueue()
		}
		
		queueItem := msgQueueItem{level, context, message}

		asnLogger.queueHasElements.L.Lock()
		defer asnLogger.queueHasElements.L.Unlock()

		asnLogger.msgQueue.PushBack(queueItem)
		asnLogger.queueHasElements.Broadcast()
	} else {
		err := errors.New(fmt.Sprintf("Queue closed! Cannot process element: %d %#v", level, message))
		reportInternalError(err)
	}
}
