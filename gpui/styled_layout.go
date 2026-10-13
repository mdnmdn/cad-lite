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

// Absolute removes the element from flex flow.
func (m *StyledMixin) Absolute() *StyledMixin { v := PositionAbsolute; m.style.Position = &v; return m }

// Absolute is StyledMixin.Absolute returning *Div to keep the chain.
func (d *Div) Absolute() *Div { d.StyledMixin.Absolute(); return d }

// Absolute is StyledMixin.Absolute returning *Text to keep the chain.
func (t *Text) Absolute() *Text { t.StyledMixin.Absolute(); return t }

// Relative restores normal flex flow.
func (m *StyledMixin) Relative() *StyledMixin { v := PositionRelative; m.style.Position = &v; return m }

// Relative is StyledMixin.Relative returning *Div to keep the chain.
func (d *Div) Relative() *Div { d.StyledMixin.Relative(); return d }

// Relative is StyledMixin.Relative returning *Text to keep the chain.
func (t *Text) Relative() *Text { t.StyledMixin.Relative(); return t }

// Top sets the absolute top offset in logical pixels.
func (m *StyledMixin) Top(v float32) *StyledMixin { l := Px(v); m.style.Top = &l; return m }

// Top is StyledMixin.Top returning *Div to keep the chain.
func (d *Div) Top(v float32) *Div { d.StyledMixin.Top(v); return d }

// Top is StyledMixin.Top returning *Text to keep the chain.
func (t *Text) Top(v float32) *Text { t.StyledMixin.Top(v); return t }

// Right sets the absolute right offset in logical pixels.
func (m *StyledMixin) Right(v float32) *StyledMixin { l := Px(v); m.style.Right = &l; return m }

// Right is StyledMixin.Right returning *Div to keep the chain.
func (d *Div) Right(v float32) *Div { d.StyledMixin.Right(v); return d }

// Right is StyledMixin.Right returning *Text to keep the chain.
func (t *Text) Right(v float32) *Text { t.StyledMixin.Right(v); return t }

// Bottom sets the absolute bottom offset in logical pixels.
func (m *StyledMixin) Bottom(v float32) *StyledMixin { l := Px(v); m.style.Bottom = &l; return m }

// Bottom is StyledMixin.Bottom returning *Div to keep the chain.
func (d *Div) Bottom(v float32) *Div { d.StyledMixin.Bottom(v); return d }

// Bottom is StyledMixin.Bottom returning *Text to keep the chain.
func (t *Text) Bottom(v float32) *Text { t.StyledMixin.Bottom(v); return t }

// Left sets the absolute left offset in logical pixels.
func (m *StyledMixin) Left(v float32) *StyledMixin { l := Px(v); m.style.Left = &l; return m }

// Left is StyledMixin.Left returning *Div to keep the chain.
func (d *Div) Left(v float32) *Div { d.StyledMixin.Left(v); return d }

// Left is StyledMixin.Left returning *Text to keep the chain.
func (t *Text) Left(v float32) *Text { t.StyledMixin.Left(v); return t }

// Inset sets absolute offsets on all four edges.
func (m *StyledMixin) Inset(v float32) *StyledMixin { m.Top(v).Right(v).Bottom(v).Left(v); return m }

// Inset is StyledMixin.Inset returning *Div to keep the chain.
func (d *Div) Inset(v float32) *Div { d.StyledMixin.Inset(v); return d }

// Inset is StyledMixin.Inset returning *Text to keep the chain.
func (t *Text) Inset(v float32) *Text { t.StyledMixin.Inset(v); return t }

// InsetX sets absolute offsets on the left and right edges.
func (m *StyledMixin) InsetX(v float32) *StyledMixin { m.Left(v).Right(v); return m }

// InsetX is StyledMixin.InsetX returning *Div to keep the chain.
func (d *Div) InsetX(v float32) *Div { d.StyledMixin.InsetX(v); return d }

// InsetX is StyledMixin.InsetX returning *Text to keep the chain.
func (t *Text) InsetX(v float32) *Text { t.StyledMixin.InsetX(v); return t }

// InsetY sets absolute offsets on the top and bottom edges.
func (m *StyledMixin) InsetY(v float32) *StyledMixin { m.Top(v).Bottom(v); return m }

// InsetY is StyledMixin.InsetY returning *Div to keep the chain.
func (d *Div) InsetY(v float32) *Div { d.StyledMixin.InsetY(v); return d }

// InsetY is StyledMixin.InsetY returning *Text to keep the chain.
func (t *Text) InsetY(v float32) *Text { t.StyledMixin.InsetY(v); return t }

// FlexWrap enables wrapping into multiple flex lines.
func (m *StyledMixin) FlexWrap() *StyledMixin { v := true; m.style.Wrap = &v; return m }

// FlexWrap is StyledMixin.FlexWrap returning *Div to keep the chain.
func (d *Div) FlexWrap() *Div { d.StyledMixin.FlexWrap(); return d }

// FlexWrap is StyledMixin.FlexWrap returning *Text to keep the chain.
func (t *Text) FlexWrap() *Text { t.StyledMixin.FlexWrap(); return t }

// FlexNoWrap restores a single flex line.
func (m *StyledMixin) FlexNoWrap() *StyledMixin { v := false; m.style.Wrap = &v; return m }

// FlexNoWrap is StyledMixin.FlexNoWrap returning *Div to keep the chain.
func (d *Div) FlexNoWrap() *Div { d.StyledMixin.FlexNoWrap(); return d }

// FlexNoWrap is StyledMixin.FlexNoWrap returning *Text to keep the chain.
func (t *Text) FlexNoWrap() *Text { t.StyledMixin.FlexNoWrap(); return t }

// RowGap sets the physical vertical gap in logical pixels.
func (m *StyledMixin) RowGap(v float32) *StyledMixin { l := Px(v); m.style.RowGap = &l; return m }

// RowGap is StyledMixin.RowGap returning *Div to keep the chain.
func (d *Div) RowGap(v float32) *Div { d.StyledMixin.RowGap(v); return d }

// RowGap is StyledMixin.RowGap returning *Text to keep the chain.
func (t *Text) RowGap(v float32) *Text { t.StyledMixin.RowGap(v); return t }

// ColumnGap sets the physical horizontal gap in logical pixels.
func (m *StyledMixin) ColumnGap(v float32) *StyledMixin { l := Px(v); m.style.ColumnGap = &l; return m }

// ColumnGap is StyledMixin.ColumnGap returning *Div to keep the chain.
func (d *Div) ColumnGap(v float32) *Div { d.StyledMixin.ColumnGap(v); return d }

// ColumnGap is StyledMixin.ColumnGap returning *Text to keep the chain.
func (t *Text) ColumnGap(v float32) *Text { t.StyledMixin.ColumnGap(v); return t }

// ContentStretch sets wrapped-line alignment to Stretch.
func (m *StyledMixin) ContentStretch() *StyledMixin {
	v := AlignContentStretch
	m.style.AlignContent = &v
	return m
}

// ContentStretch is StyledMixin.ContentStretch returning *Div to keep the chain.
func (d *Div) ContentStretch() *Div { d.StyledMixin.ContentStretch(); return d }

// ContentStretch is StyledMixin.ContentStretch returning *Text to keep the chain.
func (t *Text) ContentStretch() *Text { t.StyledMixin.ContentStretch(); return t }

// ContentStart sets wrapped-line alignment to Start.
func (m *StyledMixin) ContentStart() *StyledMixin {
	v := AlignContentStart
	m.style.AlignContent = &v
	return m
}

// ContentStart is StyledMixin.ContentStart returning *Div to keep the chain.
func (d *Div) ContentStart() *Div { d.StyledMixin.ContentStart(); return d }

// ContentStart is StyledMixin.ContentStart returning *Text to keep the chain.
func (t *Text) ContentStart() *Text { t.StyledMixin.ContentStart(); return t }

// ContentCenter sets wrapped-line alignment to Center.
func (m *StyledMixin) ContentCenter() *StyledMixin {
	v := AlignContentCenter
	m.style.AlignContent = &v
	return m
}

// ContentCenter is StyledMixin.ContentCenter returning *Div to keep the chain.
func (d *Div) ContentCenter() *Div { d.StyledMixin.ContentCenter(); return d }

// ContentCenter is StyledMixin.ContentCenter returning *Text to keep the chain.
func (t *Text) ContentCenter() *Text { t.StyledMixin.ContentCenter(); return t }

// ContentEnd sets wrapped-line alignment to End.
func (m *StyledMixin) ContentEnd() *StyledMixin {
	v := AlignContentEnd
	m.style.AlignContent = &v
	return m
}

// ContentEnd is StyledMixin.ContentEnd returning *Div to keep the chain.
func (d *Div) ContentEnd() *Div { d.StyledMixin.ContentEnd(); return d }

// ContentEnd is StyledMixin.ContentEnd returning *Text to keep the chain.
func (t *Text) ContentEnd() *Text { t.StyledMixin.ContentEnd(); return t }

// ContentBetween sets wrapped-line alignment to SpaceBetween.
func (m *StyledMixin) ContentBetween() *StyledMixin {
	v := AlignContentSpaceBetween
	m.style.AlignContent = &v
	return m
}

// ContentBetween is StyledMixin.ContentBetween returning *Div to keep the chain.
func (d *Div) ContentBetween() *Div { d.StyledMixin.ContentBetween(); return d }

// ContentBetween is StyledMixin.ContentBetween returning *Text to keep the chain.
func (t *Text) ContentBetween() *Text { t.StyledMixin.ContentBetween(); return t }

// ContentAround sets wrapped-line alignment to SpaceAround.
func (m *StyledMixin) ContentAround() *StyledMixin {
	v := AlignContentSpaceAround
	m.style.AlignContent = &v
	return m
}

// ContentAround is StyledMixin.ContentAround returning *Div to keep the chain.
func (d *Div) ContentAround() *Div { d.StyledMixin.ContentAround(); return d }

// ContentAround is StyledMixin.ContentAround returning *Text to keep the chain.
func (t *Text) ContentAround() *Text { t.StyledMixin.ContentAround(); return t }
