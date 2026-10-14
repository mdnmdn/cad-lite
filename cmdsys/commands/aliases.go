package commands

// Aliases are the default command aliases, in acad.pgp format. The text is frozen: a command
// package registers its own aliases with r.Alias in its registerX function (the table below is
// only the aliases that predate the split).
const Aliases = `; Cad-lite default aliases (acad.pgp format)
L,     *LINE
PL,    *PLINE
C,     *CIRCLE
A,     *ARC
REC,   *RECTANG
E,     *ERASE
M,     *MOVE
CO,    *COPY
CP,    *COPY
RO,    *ROTATE
SC,    *SCALE
Z,     *ZOOM
LA,    *LAYER
DI,    *DIST
LI,    *LIST
AA,    *AREA
MEA,   *MEASUREGEOM
`
