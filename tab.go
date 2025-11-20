// tab.go
package main

import (
	"os"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
)

type Tab struct {
	a      fyne.App
	w      fyne.Window
	parent *container.AppTabs
	ti     *container.TabItem
	prog,
	totpProg *widget.ProgressBar
	cosED,
	cosSH []fyne.CanvasObject
	cancelButton,
	mainButton,
	secretButton,
	cbButton,
	deleteAllButton,
	treeButton *widget.Button
	topline,
	totpLabel *widget.Label
	entry       *widget.Entry
	entryText   string
	totpCheck   *widget.Check
	boxholder   *fyne.Container
	scroller    *container.Scroll
	fileentries sync.Map
	cancelChan,
	doneChan chan struct{}
}

func newTab(a fyne.App, w fyne.Window, parent *container.AppTabs, tl string) *Tab {
	t := Tab{
		a:      a,
		w:      w,
		parent: parent,
	}
	t.prog = widget.NewProgressBar()
	t.topline = widget.NewLabel(lp(tl))
	t.entry = widget.NewEntryWithData(binding.BindPreferenceString("secret", a.Preferences()))
	t.entryText = os.Getenv(CROC_SECRET)
	if t.entryText != "" {
		t.entry.SetText(t.entryText)
	}
	t.totpCheck = widget.NewCheckWithData("", binding.BindPreferenceBool("totp-recv", a.Preferences()))
	t.totpLabel = widget.NewLabel(TOTP)
	t.totpProg = setupTOTP(a, t.entry, t.totpCheck, t.totpLabel, &t.entryText)
	t.boxholder = container.NewVBox()
	t.scroller = container.NewVScroll(t.boxholder)
	t.deleteAllButton = widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
		fyne.Do(func() {
			if mapEmpty(&t.fileentries) {
				t.entry.SetText(t.entryText)
			} else {
				t.removeEntrys(true)
			}
		})
	})
	t.doneChan = make(chan struct{})
	t.cancelChan = make(chan struct{})
	t.cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		close(t.cancelChan)
	})
	t.cosED = append(t.cosED,
		t.entry,
		t.secretButton,
		t.totpCheck,
		t.deleteAllButton,
		t.mainButton,
		t.treeButton,
	)
	t.cosSH = append(t.cosSH,
		t.prog,
		t.cancelButton,
	)

	return &t
}

func (t *Tab) removeEntry(fpath string, del bool) {
	if del {
		remove := os.Remove
		de := "file"
		fi, _ := os.Stat(fpath)
		if fi != nil {
			if fi.IsDir() {
				remove = os.RemoveAll
				de = "dir"
			}

			if err := remove(fpath); err != nil {
				log.Errorf("remove %s %s: %v", de, fpath, err)
				return
			} else {
				log.Tracef("remove %s %s", de, fpath)
			}
		}
	}
	fe, ok := load(&t.fileentries, fpath)
	if !ok {
		return
	}
	t.fileentries.Delete(fpath)
	fyne.Do(func() {
		t.boxholder.Remove(fe)
		t.boxholder.Refresh()
		if ftw != nil {
			ftw.Close()
		}
	})
}

func (t *Tab) ready() (ok bool) {
	ok = true
	t.fileentries.Range(func(key, value interface{}) bool {
		fe := value.(*fyne.Container)
		if fe == nil {
			ok = false
			return false
		}
		if len(fe.Objects) <= feBar {
			ok = false
			return false
		}
		if fe.Objects[feBar].Visible() {
			ok = false
			return false
		}
		return true
	})
	return ok
}

func (t *Tab) showPage() {
	if t.ti == nil {
		return
	}
	if t.parent.Selected() != t.ti {
		fyne.Do(func() {
			t.parent.Select(t.ti)
		})
	}
}

func (t *Tab) removeEntrys(del bool) {
	if !del {
		t.fileentries.Clear()
		fyne.Do(func() {
			t.boxholder.RemoveAll()
			if ftw != nil {
				ftw.Close()
			}
		})
		return
	}
	forEachFileEntry(&t.fileentries, func(fpath string, fe *fyne.Container) {
		t.removeEntry(fpath, del)
	})
}
