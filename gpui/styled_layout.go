package gpui

// Forwarders for the margin and flex-basis builders, so *Div and *Text
// chains keep their concrete type.

// FlexAuto is StyledMixin.FlexAuto returning *Div to keep the chain.
func (d *Div) FlexAuto() *Div { d.StyledMixin.FlexAuto(); return d }

// FlexNone is StyledMixin.FlexNone returning *Div to keep the chain.
func (d *Div) FlexNone() *Div { d.StyledMixin.FlexNone(); return d }

// FlexBasis is StyledMixin.FlexBasis returning *Div to keep the chain.
func (d *Div) FlexBasis(v float32) *Div { d.StyledMixin.FlexBasis(v); return d }

// Mx is StyledMixin.Mx returning *Div to keep the chain.
func (d *Div) Mx(v float32) *Div { d.StyledMixin.Mx(v); return d }

// My is StyledMixin.My returning *Div to keep the chain.
func (d *Div) My(v float32) *Div { d.StyledMixin.My(v); return d }

// Mt is StyledMixin.Mt returning *Div to keep the chain.
func (d *Div) Mt(v float32) *Div { d.StyledMixin.Mt(v); return d }

// Mr is StyledMixin.Mr returning *Div to keep the chain.
func (d *Div) Mr(v float32) *Div { d.StyledMixin.Mr(v); return d }

// Mb is StyledMixin.Mb returning *Div to keep the chain.
func (d *Div) Mb(v float32) *Div { d.StyledMixin.Mb(v); return d }

// Ml is StyledMixin.Ml returning *Div to keep the chain.
func (d *Div) Ml(v float32) *Div { d.StyledMixin.Ml(v); return d }

// MAuto is StyledMixin.MAuto returning *Div to keep the chain.
func (d *Div) MAuto() *Div { d.StyledMixin.MAuto(); return d }

// MxAuto is StyledMixin.MxAuto returning *Div to keep the chain.
func (d *Div) MxAuto() *Div { d.StyledMixin.MxAuto(); return d }

// MyAuto is StyledMixin.MyAuto returning *Div to keep the chain.
func (d *Div) MyAuto() *Div { d.StyledMixin.MyAuto(); return d }

// MtAuto is StyledMixin.MtAuto returning *Div to keep the chain.
func (d *Div) MtAuto() *Div { d.StyledMixin.MtAuto(); return d }

// MrAuto is StyledMixin.MrAuto returning *Div to keep the chain.
func (d *Div) MrAuto() *Div { d.StyledMixin.MrAuto(); return d }

// MbAuto is StyledMixin.MbAuto returning *Div to keep the chain.
func (d *Div) MbAuto() *Div { d.StyledMixin.MbAuto(); return d }

// MlAuto is StyledMixin.MlAuto returning *Div to keep the chain.
func (d *Div) MlAuto() *Div { d.StyledMixin.MlAuto(); return d }

// FlexAuto is StyledMixin.FlexAuto returning *Text to keep the chain.
func (t *Text) FlexAuto() *Text { t.StyledMixin.FlexAuto(); return t }

// FlexNone is StyledMixin.FlexNone returning *Text to keep the chain.
func (t *Text) FlexNone() *Text { t.StyledMixin.FlexNone(); return t }

// FlexBasis is StyledMixin.FlexBasis returning *Text to keep the chain.
func (t *Text) FlexBasis(v float32) *Text { t.StyledMixin.FlexBasis(v); return t }

// Mx is StyledMixin.Mx returning *Text to keep the chain.
func (t *Text) Mx(v float32) *Text { t.StyledMixin.Mx(v); return t }

// My is StyledMixin.My returning *Text to keep the chain.
func (t *Text) My(v float32) *Text { t.StyledMixin.My(v); return t }

// Mt is StyledMixin.Mt returning *Text to keep the chain.
func (t *Text) Mt(v float32) *Text { t.StyledMixin.Mt(v); return t }

// Mr is StyledMixin.Mr returning *Text to keep the chain.
func (t *Text) Mr(v float32) *Text { t.StyledMixin.Mr(v); return t }

// Mb is StyledMixin.Mb returning *Text to keep the chain.
func (t *Text) Mb(v float32) *Text { t.StyledMixin.Mb(v); return t }

// Ml is StyledMixin.Ml returning *Text to keep the chain.
func (t *Text) Ml(v float32) *Text { t.StyledMixin.Ml(v); return t }

// MAuto is StyledMixin.MAuto returning *Text to keep the chain.
func (t *Text) MAuto() *Text { t.StyledMixin.MAuto(); return t }

// MxAuto is StyledMixin.MxAuto returning *Text to keep the chain.
func (t *Text) MxAuto() *Text { t.StyledMixin.MxAuto(); return t }

// MyAuto is StyledMixin.MyAuto returning *Text to keep the chain.
func (t *Text) MyAuto() *Text { t.StyledMixin.MyAuto(); return t }

// MtAuto is StyledMixin.MtAuto returning *Text to keep the chain.
func (t *Text) MtAuto() *Text { t.StyledMixin.MtAuto(); return t }

// MrAuto is StyledMixin.MrAuto returning *Text to keep the chain.
func (t *Text) MrAuto() *Text { t.StyledMixin.MrAuto(); return t }

// MbAuto is StyledMixin.MbAuto returning *Text to keep the chain.
func (t *Text) MbAuto() *Text { t.StyledMixin.MbAuto(); return t }

// MlAuto is StyledMixin.MlAuto returning *Text to keep the chain.
func (t *Text) MlAuto() *Text { t.StyledMixin.MlAuto(); return t }
