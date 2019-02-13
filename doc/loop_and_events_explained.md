# Loops and Events Explained

## The Basics
The vnet event manager is implemented in elib.  First on the data structure, at a high level.

Node is the top structure.  Every Node has a loop engine that's of structure *Loop.  Within a Loop there are dataPollers and/or eventHandlers that are go routines spun off in parallel at the start of GOES to run in their own infinite loops.
```
type Node struct {
        l                       *Loop
        name                    string
        noder                   Noder
        index                   uint
        ft                      fromToNode
        activePollerIndex       uint
        initOnce                sync.Once
        startEventHandlerOnce   sync.Once
        initWg                  sync.WaitGroup
        Next                    []string
        nextNodes               nextNodeVec
        nextIndexByNodeName     map[string]uint
        inputStats, outputStats nodeStats
        elogNodeName            elog.StringRef
        e                       eventNode
        s                       nodeState
}
```
The loop structure itself has also has a method Run that runs in an infinite loop
```
func (l *Loop) Run() {
```
that includes in it an infinite loop
```
        for {
                if quit := l.doEvents(); quit {
                        dbgelib.Loop.Log("quit loop !!!!!!!!!!!!")
                        break
                }
                l.doPollers()
        }
```
When a Node is initialized and its Loop started, it'll run indefinitely alternating between doEvents (eventHandler) and doPollers (dataPollers)

In general, and for the purpose of this doc, there are 4 Nodes that kick off loop engines at the start of GOES

**pg0** - No eventHandler.  **Kicks off a dataPoller**, but doesn't appear to have any activity.  dataPoller starts a waitLoop and waits forever.

**unix-rx** - No eventHandler.  **Kicks off a dataPoller** but doesn't appear to have any activity.  dataPoller starts a waitLoop and waits forever.

**fe1-rx** - No eventHander.  **Kicks off a dataPoller.**  Flurry of activity with GOES start, and then waits in the waitLoop for a long time.

**vnet** - No dataPoller.  **Kicks off an eventHandler**.  This eventHander is the workhorse of vnet that's scheduling almost all of the events requested by various parts of vnet.

So after GOES start, there will be 8 infinite loops running.  4 Loop.Run, one of each node.  3 dataPoll. 1 eventHandler.

The 3 dataPoll go dormant fairly quickly after GOES start, and waits (with no timeout) on item to show up on a go channel.

## eventHandler, nodeEvents, actors
For the remainder of this doc, the focus will be on the eventHandler.

The eventHander is a method of Loop that's defined in elib/loop/event.go
```
func (l *Loop) eventHandler(r Noder) {
```
Within the eventHandler function is an infinite for loop that pulls event out of a channel and execute it.
```
        for {
                n.log(d, event_elog_node_wait)
                {
                        t := 30 * time.Second
                        if n.currentEvent.e == nil {
                                n.ft.waitLoop_with_timeout(t, d.name+"(eventHandler)", "empty nodeEvent", n.rxEvents)
                        } else {
                                n.ft.waitLoop_with_timeout(t, d.name+"(eventHandler)", fmt.Sprintf("%v", n.currentEvent.e), n.rxEvents)
                        }
                }
                n.log(d, event_elog_node_wake)
                var e *nodeEvent
                doneGetEvent := false
                for !doneGetEvent {
                        select {
                        case e = <-n.rxEvents:
                                doneGetEvent = true
                        case <-time.After(1 * time.Second):
                                dbgelib.Loop.Logf("eventHandler get event timed out len(rxEvent) = %v", len(n.rxEvents))
                        }
                }
                if poller_panics && e.d != d {
                        dbgelib.Loop.Logf("eventHandler panic expected node %s got %s: %p %v", d.name, e.d.name, e, e)
                        panic(fmt.Errorf("expected node %s got %s: %p %v", d.name, e.d.name, e, e))
                }
                n.currentEvent.e = e
                dbgelib.Loop.Logf("==============>do event %v", e)
                e.do()
                d.eventDone()
        }
```

At the heart of an even is a nodeEvent with an actor
```
type nodeEvent struct {
        l          *Loop
        d          *Node
        actor      event.Actor
        time       cpu.Time
        caller     elog.Caller
        prev_actor string
}
```
The actor is what the event actually does, and it is an interface type that just requires a function, EventAction(), and a String() for the name of the actor.
```
type Actor interface {
        EventAction()
        String() string
}
```
So any structure in vnet Node can be turned into an event by having an EventAction() and a String() methods.

To get an event into the event queue, call SignalEvent() from Node with the actor as an argument.  That actor will be turned into a nodeEvent and placed into the channel called "event" in the Loop, from which the eventHandler will *eventually* execute from.
```
func (l *Loop) signalEvent(le *nodeEvent) {
        select {
        case l.events <- le:
                dbgelib.Loop.Logf("signalEvent(success) %v, len(l.events) = %v ", le.actor, len(l.events))
        default:
                dbgelib.Loop.Logf("signalEvent(queue full, try later) %v, len(l.events) = %v ", le.actor, len(l.events))
                l.signalEventAfter(le, 0)
        }
}
func (n *Node) SignalEventp(a event.Actor, dst Noder, p elog.PointerToFirstArg) {
        e := n.l.getLoopEvent(a, dst, p)
        n.l.signalEvent(e)
}

// SignalEvent adds event whose action will be called on the next loop iteration.
func (n *Node) SignalEvent(e event.Actor, dst Noder) {
        n.SignalEventp(e, dst, elog.PointerToFirstArg(&n))
}
```
Note that signalEvent puts a nodeEvent into the channel Loop.events, but evenHandler grabs a nodeEvents from the channel eventNode.rxEvent.  Yes, Node.e is an eventNode, not to be confused with nodeEvent.
```
type eventNode struct {
        // Index in active vector if active else ^uint(0).
        activeIndex uint

        activeCount uint32

        // Handler sequence to identify events in event log.
        sequence       uint32
        queue_sequence uint32

        rxEvents chan *nodeEvent

        ft fromToNode

        currentEvent Event
        s            eventNodeState
        eventStats   nodeStats
        activateEvent

        hasHandler bool //true if already has an eventHandler                                                                                                                                                   }
```
Within a eventNode struct is the channel rxEvent of nodeEvent.  There is also a currentEvent of the type Event, which is just a pointer to the current nodeEvent.
```
type Event struct {
        e *nodeEvent
}
```
## An Simple Example
Let's take a simple example and see what actually happens between when an event is signaled to when it is executed.

In platinasystems.com/vnet-platina-mk1 there is struct called
```
type  unresolvedArper  struct {
```
It has a method String() and a method EventAction(), so per the definition of type elib/event.Actor, unresolvedArper is an Actor.   We will not go into the detailed of a timed event for now, but suffice to say that at some point a signaEvent is called, and a nodeEvent, whoes actor is unresolvedArper, is put into the channel of Loop.event of the Node vnet.

Node vnet's Loop engine has the infinite for loop that alternates between doEvents() and doPollers().  During a doEvents() cycle, an nodeEvent is pulled out of the Loop.event channel.  Note that nodeEvent itself also has a method EventAction(), not to be confused with the EventAction() of nodeEvent's actor.

Part of the actions of the nodeEvent.EvenAction() is to put this nodeEvent into the rxEvent channel of the eventNode of the Node ...  It's OK, take a few minutes to read that a few times.

Recall that the Node vnet spins out an eventHandler go routine during init that's running continuously waiting for nodeEvent to show up at eventNode.rxEvent channel.  When it gets one, the EventAction of the eventNode's actor is executed

To summarize:
- We make unresolvedArper struct into an actor by giving it an EventAction() and a String() method.
- We call SignalEvent at some point because we want an event scheduled that will invoke unresolvedArper.EventAction()
- SignalEvent (after a few other function calls) eventually makes a nodeEvent with an unresolvedArper as an actor and put this nodeEvent into the Node vnet's Loop.event channel.
- doEvents(), which is invoked by Loop.Run continuously alternating between doEvents() and doPollers(), pulls this nodeEvent from the Loop.event and executes the EventAction of a nodeEvent (not the EventAction of its actor).
- The EventAction of any nodeEvent is to put the nodeEvent into the eventNode's rxEvent channel.
- The eventHandler, running as a go routine, waiting for nodeEvents to show up at its eventNode's rxEvent channel, grabs the nodeEvent and executes the its actor's EventAction which in this example is unresolvedArper.EventAction().

Simple enough?  There is more...

## waitNode, waitLoop
Before the eventHandler grabs something from rxEvent to execute, it calls waitLoop_with_timeout which waits for a signal from signalNode() or until a 30sec timer expires.

signalNode() is invoked in the doEvents() function, and doEvents() is part of the infinite loop of Loop.Run that alternates between doEvents() and doPollers().  That means periodically and frequently signalNode() will occur unless vnet has crashed or some process has hung.

Within doEvents(), signalNode() is called after the nodeEvent's EventAction is executed, meaning the same nodeEvent is now placed into the rxEvent channel.  signalNode() then allows eventHandler to exit its waitLoop and grab a nodeEvent from rxEvent and execute its actor.  Note there may be a queue of nodeEvents so what evenHandler grabs is not necessarily the immediate nodEvent that doEvents() just put in.

Within doEvents(), after signalNode(), a call is made to waitNode_with_timeout.  waitNode_with_timeout sits and waits for a signal from signalLoop() or until a 30sec timer expires.

signalLoop() can be invoked from several places, but the most common is from eventDone(), which is the last thing eventHandler does after it executes the actor's EventAction() and before it starts the next loop.

In summary, waitNode and waitLoop are ways for the Loop.Run go routine and the eventHandler go routine, running in parallel, to signal each other when each is done.  In normal operation, they ping-pong back and forth always alternating.

- doEvent() pulls a nodeEvent out of Loop.events channel, puts it on to the eventNode's rxEvent channel and signals evenHandler by calling signalNode().  doEvent() will then go into a waitNode cycles until a signalLoop() signal unlock waitNode.
- evenHandler, after getting a signalNode() that unlocks waitLoop, pulls a nodeEvent out of the rxEvent channel, execute the nodeEvent.actor's EventAction, and when done signals doEvent() by calling signalLoop.  eventHandler then goes into waitLoop cycle and waits for signalNode() to unlock it.
- repeat

The other benefit of of waitLoop and waitNode is that by using channels that are blocking (until time out anyway), both go routines call into go scheduler regularly and allows other go routines to be scheduled.

## Suspend, Resume
Sometimes something substantial needs to be executed right away instead of scheduled into event queues.  When such an occasion arises, it may be desirable to "suspend" Loop.

Note that signalLoop actually takes a bool argument, and the way it "signals" waitNode is by putting the bool into a channel.
```
func (x *fromToNode) signalLoop(v bool) { x.fromNode <- v }
```
In the straightforward example above, signalLoop came from eventDone, and has argument true, i.e. signalLoop(true).  That means the event is actually completed.

In the case an event is suspended by calling SuspendWTimeou(), the suspend function sends a signalLoop(false) and then goes into its own waitLoop_with_timeout cycle (with a different timer).  If Loop.Run is in the middle of a doEvent() waitNode cycle waiting for evenHandler to complete the actor.EventAction, signalLoop(false) will break out of the waitNode_with_timeout immediately, but with a "false" value, meaning event did not complete normally yet.  This will put the Node into an inactive state, i.e. moving the Node from list of activeNodes to list of inactiveNodes.  Note that evenHandler will still complete the actor.EventAction() as it is running in a separate go routine.

When a Node is suspended or inactive, doEvent() will not signalNode, so after eventHandler finishes and goes into waitLoop_with_timeout, it will not be getting a signal from doEvent to get the next nodeEvent from rxEvent channel.   At this point nodeEvent can still be added to Loop.events.  Loop.Run is still running and doEvent() is still adding events to rxEvent, but without waiting for eventHandler to finish executing an event.  If no other signal comes to undo the suspension, the waitLoop_with_timeout initiated by eventHandler will eventually timeout and crash vnet.

Wherever SuspendWTimeout() is called to suspend a Node, at some point it will have to call Resume() to put the node back into active state.  Presumably Resume() is called before either the waitLoop_with_timeout() initiated by SuspendWTimeout or the waitLoop_with_timeout() initiated by eventHandler expires (crashes vnet either expires).  When Resume() is called, it will unlock waitLoop_with_timeout() initiated by SuspendWTimeout so that SuspendWTimeout completes and wherever the function was called from can continue executing the next instruction.   Resume() also sets the actor of the nodeEvent that was suspended to nil.  That's the indication that the nodeEvent is resumed.
```
func (e *nodeEvent) isResume() bool { return e.actor == nil }
```
Resume also puts the nodeEvent back into the Loop.event channel with the nil actor.  It's ignored later in doEvent (through functions that it calls) because of the nil actor.

Next time around on doEvent, the Node will be set back to active. signalNode() will be called and evenHandler will continue again and all is back to normal.  When there are no more new nodeEvent being added, doEvents() will continue with empty nodeEvent (the default select makes non-blocking getting from l.events) and signalNode at each loop of Loop.Run so eventHandler will keep pulling nodeEvents out of rxEvent and execute to catch up on the backlog.

```
func (l *Loop) doEventNoWait() (quit *quitEvent) {
        select {
        default: // nothing to do
        case e := <-l.events:
                quit = l.doNodeEvent(e)
        }
        return
}
```
A common place to see SuspendWTimeout called is in fe1's sbus_dma access.  subs_dma suspends Node vnet (with 2 second timer), and executes dma commands.  When the dma is completed, Resume is called.

## Sample Logs
Here's are snipped from actual debug logs showing some of the actions described above
```
Feb  8 06:25:03 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Loop).eventHandler() ==============>do event unresolvedArper ping
Feb  8 06:25:03 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Node).eventDone() signalLoop(true) from vnet actor unresolvedArper ping
Feb  8 06:25:03 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitLoop_with_timeout() start, node: vnet(eventHandler), actor: empty nodeEvent, len(chan)=0 dur=30s num waitLoop = 4
Feb  8 06:25:03 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitNode_with_timeout() done, node: vnet(doEvents), actor: empty nodeEvent, len(chan)=0, in 33.925µs
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Loop).doEvents() signalNode from vnet actor <nil>
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitLoop_with_timeout() github.com/platinasystems/elib/loop.(*fromToNode).waitNode_with_timeout() done, node: vnet(eventHandler), actor: empty nodeEvent, len(chan)=1, in 980.514108ms
Feb  8 06:25:04 invader34 goes.vnetd[25456]: start, node: vnet(doEvents), actor: empty nodeEvent, len(chan)=1, dur=30s, num waitNode = 1
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Loop).eventHandler() ==============>do event redis stats poller sequence 27133
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Event).SuspendWTimeout() signalLoop(false) from vnet actor redis stats poller sequence 27133
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Event).Resume() put event redis stats poller sequence 27133 back and set actor to nil, len(d.l.events) = 1
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitNode_with_timeout() done, node: vnet(doEvents), actor: empty nodeEvent, len(chan)=0, in 381.465µs
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Loop).doEvents() signalNode from vnet actor nil(was redis stats poller sequence 27133)
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitNode_with_timeout() start, node: vnet(doEvents), actor: nil(was redis stats poller sequence 27133), len(chan)=0, dur=30s, num waitNode = 1
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitLoop_with_timeout() start, node: vnet(suspend), actor: nil(was redis stats poller sequence 27133), len(chan)=0 dur=2s num waitLoop = 4
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitLoop_with_timeout() done, node: vnet(suspend), actor: nil(was redis stats poller sequence 27133), len(chan)=0, in 3.173µs
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Node).eventDone() signalLoop(true) from vnet actor nil(was redis stats poller sequence 27133)
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitLoop_with_timeout() start, node: vnet(eventHandler), actor: empty nodeEvent, len(chan)=0 dur=30s num waitLoop = 4
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitNode_with_timeout() done, node: vnet(doEvents), actor: nil(was redis stats poller sequence 27133), len(chan)=0, in 17.276637ms
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*Loop).doEvents() signalNode from vnet actor <nil>
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitNode_with_timeout() start, node: vnet(doEvents), actor: empty nodeEvent, len(chan)=1, dur=30s, num waitNode = 1
Feb  8 06:25:04 invader34 goes.vnetd[25456]: github.com/platinasystems/elib/loop.(*fromToNode).waitLoop_with_timeout() done, node: vnet(eventHandler), actor: empty nodeEvent, len(chan)=1, in 1.913477ms
```
