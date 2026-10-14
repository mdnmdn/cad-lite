package ui

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"

	"agentmanager"
	"agentmanager/transcript"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Limits on the images attached to an agent prompt. An image larger than agentImageMaxEdge on
// either side is scaled down to fit; one that is still over agentImageMaxBytes once encoded is
// refused, as is a file over agentImageMaxFile before it is even decoded.
const (
	agentImageMaxEdge  = 2048
	agentImageMaxBytes = 5 << 20
	agentImageMaxFile  = 40 << 20
	agentImageMaxCount = 6
	// agentThumbEdge is the longest side of a stored and shown preview, in pixels.
	agentThumbEdge = 96
)

// agentImage is an image waiting in the composer, or sent: the bytes the agent gets and a preview.
type agentImage struct {
	// name is the file's base name, "" for a pasted image.
	name string
	img  agentmanager.Image
	// w and h are the pixel size of what is sent.
	w, h  int
	thumb *image.NRGBA
}

var (
	errImageType   = errors.New("unsupported image type")
	errImageLarge  = errors.New("image too large")
	errImagePixels = errors.New("image has too many pixels")
)

// agentImageMaxPixels refuses an image before decoding it when its header announces more pixels:
// a small file can claim a huge size, and decoding allocates for all of it.
const agentImageMaxPixels = 50_000_000

// imageConfig reads the size an image's header announces.
func imageConfig(mime string, data []byte) (image.Config, error) {
	r := bytes.NewReader(data)
	switch mime {
	case "image/png":
		return png.DecodeConfig(r)
	case "image/jpeg":
		return jpeg.DecodeConfig(r)
	case "image/gif":
		return gif.DecodeConfig(r)
	case "image/webp":
		return webp.DecodeConfig(r)
	}
	return image.Config{}, errImageType
}

// imageMIME is data's type when it is one the agents read: PNG, JPEG, GIF or WebP.
func imageMIME(data []byte) (string, bool) {
	switch m := http.DetectContentType(data); m {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return m, true
	}
	return "", false
}

func decodeImage(mime string, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch mime {
	case "image/png":
		return png.Decode(r)
	case "image/jpeg":
		return jpeg.Decode(r)
	case "image/gif":
		return gif.Decode(r)
	case "image/webp":
		return webp.Decode(r)
	}
	return nil, errImageType
}

// fit is w x h scaled down, keeping the aspect ratio, so neither side exceeds edge.
func fit(w, h, edge int) (int, int) {
	if w <= edge && h <= edge {
		return w, h
	}
	if w >= h {
		return edge, max(1, h*edge/w)
	}
	return max(1, w*edge/h), edge
}

func scaled(src image.Image, w, h int, q xdraw.Interpolator) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	q.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}

// newAgentImage checks and prepares data (a file's or the clipboard's bytes) for a prompt: the
// type must be PNG, JPEG, GIF or WebP; an image over agentImageMaxEdge is scaled down and encoded
// again (JPEG stays JPEG, everything else becomes PNG, or JPEG when an opaque PNG would exceed
// agentImageMaxBytes); the result must fit agentImageMaxBytes.
func newAgentImage(name string, data []byte) (*agentImage, error) {
	mime, ok := imageMIME(data)
	if !ok {
		return nil, errImageType
	}
	cfg, err := imageConfig(mime, data)
	if err != nil {
		return nil, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > agentImageMaxPixels {
		return nil, errImagePixels
	}
	src, err := decodeImage(mime, data)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := fit(b.Dx(), b.Dy(), agentImageMaxEdge)
	out := agentmanager.Image{MIME: mime, Data: data}
	if w != b.Dx() || h != b.Dy() || len(data) > agentImageMaxBytes {
		small := scaled(src, w, h, xdraw.CatmullRom)
		var buf bytes.Buffer
		if mime == "image/jpeg" {
			err = jpeg.Encode(&buf, small, &jpeg.Options{Quality: 90})
		} else {
			mime = "image/png"
			err = png.Encode(&buf, small)
			if err == nil && buf.Len() > agentImageMaxBytes && opaque(small) {
				buf.Reset()
				mime = "image/jpeg"
				err = jpeg.Encode(&buf, small, &jpeg.Options{Quality: 85})
			}
		}
		if err != nil {
			return nil, err
		}
		out = agentmanager.Image{MIME: mime, Data: buf.Bytes()}
		src = small
	}
	if len(out.Data) > agentImageMaxBytes {
		return nil, errImageLarge
	}
	tw, th := fit(w, h, agentThumbEdge)
	return &agentImage{name: name, img: out, w: w, h: h, thumb: scaled(src, tw, th, xdraw.ApproxBiLinear)}, nil
}

// loadAgentImage reads the file at path as an agent image.
func loadAgentImage(path string) (*agentImage, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, errImageType
	}
	if st.Size() > agentImageMaxFile {
		return nil, errImageLarge
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return newAgentImage(filepath.Base(path), data)
}

// attachment is what the transcript and the store keep of an image: its name, type, size and a
// PNG of the preview (the full image is not stored; the agent's own session holds it).
func (a *agentImage) attachment() transcript.Attachment {
	var buf bytes.Buffer
	var thumb []byte
	if a.thumb != nil && png.Encode(&buf, a.thumb) == nil {
		thumb = buf.Bytes()
	}
	return transcript.Attachment{Name: a.name, MIME: a.img.MIME, Width: a.w, Height: a.h, Thumb: thumb}
}

// label is the image's name for chips and messages.
func (a *agentImage) label() string {
	if a.name != "" {
		return a.name
	}
	return i18n.T("ui.agent.image.pasted")
}

// visionOf says whether tab t's model takes images: no when its live session refuses images or the
// model is listed as text-only, yes when the model is listed as reading images or the harness is
// Claude, unknown otherwise (an attachment is then allowed and the agent has the last word).
func (p *AgentPanel) visionOf(t *agentTab) agentmanager.Vision {
	if t.sess != nil {
		if ip, ok := t.sess.(agentmanager.ImagePrompter); !ok || !ip.AcceptsImages() {
			return agentmanager.VisionNo
		}
	}
	h := p.harnessOf(t)
	id := p.modelFor(t, h)
	if id == "" {
		id = t.mdl.current
	}
	for _, o := range t.mdl.offered {
		if o.ID == id && o.Vision != agentmanager.VisionUnknown {
			return o.Vision
		}
	}
	if h == agentmanager.Claude {
		return agentmanager.VisionYes
	}
	return agentmanager.VisionUnknown
}

// note shows a warning in tab t's transcript without storing it.
func (p *AgentPanel) note(t *agentTab, msg string) {
	t.tr.Apply(agentmanager.Log{Level: "warn", Message: msg})
}

// imageError is the message for an image that cannot be attached.
func imageError(label string, err error) string {
	switch {
	case errors.Is(err, errImageType):
		return i18n.T("ui.agent.image.unsupported", label)
	case errors.Is(err, errImageLarge):
		return i18n.T("ui.agent.image.too_large", label, agentImageMaxBytes>>20)
	case errors.Is(err, errImagePixels):
		return i18n.T("ui.agent.image.too_many_pixels", label, agentImageMaxPixels/1_000_000)
	}
	return i18n.T("ui.agent.image.unreadable", label, err)
}

// attach adds images to the active tab's composer, refusing them all when its model does not read
// images and those beyond agentImageMaxCount.
func (p *AgentPanel) attach(imgs []*agentImage, w *gpui.Window) {
	t := p.cur
	if len(imgs) == 0 {
		return
	}
	if p.visionOf(t) == agentmanager.VisionNo {
		p.note(t, i18n.T("ui.agent.image.no_vision", p.modelLabel(t)))
		w.Refresh()
		return
	}
	for _, im := range imgs {
		if len(t.images) >= agentImageMaxCount {
			p.note(t, i18n.T("ui.agent.image.too_many", agentImageMaxCount))
			break
		}
		t.images = append(t.images, im)
	}
	w.Refresh()
}

// AttachFiles attaches the image files at paths to the active tab's next prompt, as a drop on the
// panel does; files that are not images, too large or unreadable are reported in the transcript.
func (p *AgentPanel) AttachFiles(paths []string, w *gpui.Window) {
	p.ensureLoaded()
	var ok []*agentImage
	for _, path := range paths {
		im, err := loadAgentImage(path)
		if err != nil {
			p.note(p.cur, imageError(filepath.Base(path), err))
			continue
		}
		ok = append(ok, im)
	}
	p.attach(ok, w)
	w.Refresh()
}

// AttachImage attaches encoded image data (a pasted image) to the active tab's next prompt.
func (p *AgentPanel) AttachImage(data []byte, w *gpui.Window) {
	p.ensureLoaded()
	im, err := newAgentImage("", data)
	if err != nil {
		p.note(p.cur, imageError(i18n.T("ui.agent.image.pasted"), err))
		w.Refresh()
		return
	}
	p.attach([]*agentImage{im}, w)
}

// Attachments returns the names of the images waiting in the active tab's composer ("" for a
// pasted one), for tests.
func (p *AgentPanel) Attachments() []string {
	var out []string
	for _, im := range p.cur.images {
		out = append(out, im.name)
	}
	return out
}

// RemoveAttachment takes the i-th image off the active tab's composer.
func (p *AgentPanel) RemoveAttachment(i int, w *gpui.Window) {
	t := p.cur
	if i >= 0 && i < len(t.images) {
		t.images = append(t.images[:i:i], t.images[i+1:]...)
	}
	w.Refresh()
}

// onFileDrop takes files dropped on the panel as image attachments; a drop on the panel never
// reaches the workspace, so it never opens a drawing.
func (p *AgentPanel) onFileDrop(e *gpui.FileDropEvent, w *gpui.Window, _ *gpui.App) {
	w.StopPropagation()
	p.AttachFiles(e.Paths, w)
}

// onImagePaste takes a pasted image as an attachment.
func (p *AgentPanel) onImagePaste(e *gpui.ImagePasteEvent, w *gpui.Window, _ *gpui.App) {
	w.StopPropagation()
	p.AttachImage(e.Data, w)
}

// thumbView paints img at most size logical pixels on its longer side, centred in a size x size box.
func thumbView(img *image.NRGBA, size float32) *gpui.Div {
	box := gpui.NewDiv().SizeWH(size, size).FlexShrink(0).Rounded(4).Bg(uiBg).OverflowHidden()
	if img == nil {
		return box
	}
	iw, ih := float32(img.Rect.Dx()), float32(img.Rect.Dy())
	k := size / max(iw, ih)
	w, h := iw*k, ih*k
	cv := gpui.NewCanvas(func(b gpui.Bounds, win *gpui.Window, _ *gpui.App) {
		ox := b.Origin.X + gpui.Pixels(size-w)/2
		oy := b.Origin.Y + gpui.Pixels(size-h)/2
		win.PaintImage(gpui.NewBounds(float32(ox), float32(oy), w, h), img)
	})
	cv.SizeWH(size, size)
	return box.Child(cv)
}

// decodeThumb decodes a stored PNG preview; nil when there is none or it is broken.
func decodeThumb(b []byte) *image.NRGBA {
	if len(b) == 0 {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	n := image.NewNRGBA(img.Bounds())
	draw.Draw(n, n.Bounds(), img, img.Bounds().Min, draw.Src)
	return n
}

// imageChips is the row of images waiting in the composer: preview, name, size and a remove button.
func (p *AgentPanel) imageChips() *gpui.Div {
	row := gpui.NewDiv().ID("agent-images").FlexRow().FlexWrap().Gap(6)
	for i, im := range p.cur.images {
		i := i
		remove := gpui.NewDiv().ID(fmt.Sprintf("agent-image-remove-%d", i)).Px_(4).Rounded(4).Tooltip(i18n.T("ui.agent.image.remove_tip")).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.RemoveAttachment(i, w) }).
			Child(gpui.NewText("x").TextSize(11).TextColor(uiMuted))
		row.Child(gpui.NewDiv().ID(fmt.Sprintf("agent-image-%d", i)).FlexRow().ItemsCenter().Gap(6).P(3).Pr(2).Rounded(6).
			Bg(uiPanelAlt).Border(1, uiBorder).MinW(0).Tooltip(i18n.T("ui.agent.image.chip_tip")).
			Child(thumbView(im.thumb, 32),
				gpui.NewDiv().FlexCol().MinW(0).Child(
					gpui.NewText(clip(im.label(), 18)).TextSize(fontSmall),
					gpui.NewText(fmt.Sprintf("%d x %d", im.w, im.h)).TextSize(10).TextColor(uiMuted)),
				remove))
	}
	return row
}

// attachmentRow shows the images of a sent message: previews where stored, else the name.
func attachmentRow(as []transcript.Attachment, cache map[string]*image.NRGBA) *gpui.Div {
	row := gpui.NewDiv().FlexRow().FlexWrap().Gap(4)
	for _, a := range as {
		key := string(a.Thumb)
		img, ok := cache[key]
		if !ok {
			img = decodeThumb(a.Thumb)
			cache[key] = img
		}
		label := a.Name
		if label == "" {
			label = i18n.T("ui.agent.image.pasted")
		}
		if img != nil {
			row.Child(thumbView(img, 48).Tooltip(fmt.Sprintf("%s, %d x %d", label, a.Width, a.Height)))
			continue
		}
		row.Child(gpui.NewDiv().Px_(6).Py_(2).Rounded(4).Bg(uiPanelAlt).
			Child(gpui.NewText(fmt.Sprintf("[%s %d x %d]", clip(label, 18), a.Width, a.Height)).TextSize(10)))
	}
	return row
}
