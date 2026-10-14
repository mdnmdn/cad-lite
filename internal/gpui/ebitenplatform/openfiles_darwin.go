//go:build darwin

package ebitenplatform

import (
	"fmt"
	"log"
	"sync"

	"github.com/ebitengine/purego/objc"
)

// Apple event codes (FourCharCodes) for the open-documents event.
const (
	aeCoreEventClass   = 0x61657674 // 'aevt', kCoreEventClass
	aeOpenDocuments    = 0x6f646f63 // 'odoc', kAEOpenDocuments
	aeKeyDirectObject  = 0x2d2d2d2d // '----', keyDirectObject
	openHandlerClass   = "CadLiteOpenDocumentsHandler"
	willFinishLaunchNM = "NSApplicationWillFinishLaunchingNotification"
)

var (
	openOnce sync.Once
	openErr  error
	// openHandler and openBlock live for the whole process.
	openHandler objc.ID
	openBlock   objc.Block
)

// installOpenFilesHandler makes the open-documents Apple event (a file opened
// from Finder or dropped on the Dock icon) push its paths to q. GLFW handles no
// such event. It must run on the main thread before ebiten.RunGame: it
// installs the handler now and again when NSApplication posts
// NSApplicationWillFinishLaunchingNotification, because NSApplication installs
// its own handlers while it launches and a launch-by-document dispatches its
// event right after that notification. It is idempotent.
func installOpenFilesHandler(q *openQueue) error {
	openOnce.Do(func() { openErr = registerOpenFilesHandler(q) })
	return openErr
}

func registerOpenFilesHandler(q *openQueue) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("objc: %v", r)
		}
	}()
	nsObject := objc.GetClass("NSObject")
	if nsObject == 0 {
		return fmt.Errorf("NSObject class not found")
	}
	selHandle := objc.RegisterName("handleOpenDocuments:withReplyEvent:")
	cls, err := objc.RegisterClass(openHandlerClass, nsObject, nil, nil, []objc.MethodDef{{
		Cmd: selHandle,
		Fn: func(_ objc.ID, _ objc.SEL, event, _ objc.ID) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("gpui: open documents event: %v", r)
				}
			}()
			q.push(openEventPaths(event))
		},
	}})
	if err != nil {
		return err
	}
	selAlloc, selInit := objc.RegisterName("alloc"), objc.RegisterName("init")
	openHandler = objc.ID(cls).Send(selAlloc).Send(selInit)
	if openHandler == 0 {
		return fmt.Errorf("%s init returned nil", openHandlerClass)
	}
	register := func() error {
		mgr := objc.ID(objc.GetClass("NSAppleEventManager")).Send(objc.RegisterName("sharedAppleEventManager"))
		if mgr == 0 {
			return fmt.Errorf("NSAppleEventManager unavailable")
		}
		mgr.Send(objc.RegisterName("setEventHandler:andSelector:forEventClass:andEventID:"),
			openHandler, selHandle, uint32(aeCoreEventClass), uint32(aeOpenDocuments))
		return nil
	}
	if err := register(); err != nil {
		return err
	}
	center := objc.ID(objc.GetClass("NSNotificationCenter")).Send(objc.RegisterName("defaultCenter"))
	if center == 0 {
		return fmt.Errorf("NSNotificationCenter unavailable")
	}
	name := objc.ID(objc.GetClass("NSString")).Send(selAlloc).Send(objc.RegisterName("initWithUTF8String:"), willFinishLaunchNM)
	openBlock = objc.NewBlock(func(_ objc.Block, _ objc.ID) {
		defer func() { _ = recover() }()
		if err := register(); err != nil {
			log.Printf("gpui: open documents event unavailable: %v", err)
		}
	})
	if obs := center.Send(objc.RegisterName("addObserverForName:object:queue:usingBlock:"), name, objc.ID(0), objc.ID(0), openBlock); obs == 0 {
		return fmt.Errorf("addObserverForName returned nil")
	}
	return nil
}

// openEventPaths reads the file paths of an open-documents Apple event: its
// direct object is a list of file URL descriptors (1-based).
func openEventPaths(event objc.ID) []string {
	if event == 0 {
		return nil
	}
	list := event.Send(objc.RegisterName("paramDescriptorForKeyword:"), uint32(aeKeyDirectObject))
	if list == 0 {
		return nil
	}
	var (
		selAt   = objc.RegisterName("descriptorAtIndex:")
		selURL  = objc.RegisterName("fileURLValue")
		selPath = objc.RegisterName("path")
		selUTF8 = objc.RegisterName("UTF8String")
	)
	pathOf := func(d objc.ID) string {
		if d == 0 {
			return ""
		}
		url := d.Send(selURL)
		if url == 0 {
			return ""
		}
		p := url.Send(selPath)
		if p == 0 {
			return ""
		}
		return objc.Send[string](p, selUTF8)
	}
	n := objc.Send[int](list, objc.RegisterName("numberOfItems"))
	if n <= 0 {
		// A single file URL instead of a list.
		if p := pathOf(list); p != "" {
			return []string{p}
		}
		return nil
	}
	var out []string
	for i := 1; i <= n; i++ {
		if p := pathOf(list.Send(selAt, i)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
