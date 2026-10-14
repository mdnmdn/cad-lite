# DWG test fixtures

Every file here is copied unchanged from the `samples/` folder of
[ACadSharp](https://github.com/DomCR/ACadSharp) (commit `31bafd2932092d3f3fa5a4f27486b4d78042f727`),
which is MIT licensed, Copyright (c) 2021 Albert Domenech. The licence text is in
`../LICENSE-ACadSharp`.

| File | Origin in ACadSharp | Version | Size |
|---|---|---|---|
| `sample_AC1014.dwg` | `samples/sample_AC1014.dwg` | R14 | 1.3 MB |
| `sample_AC1015.dwg` | `samples/sample_AC1015.dwg` | R2000 | 1.3 MB |
| `sample_AC1018.dwg` | `samples/sample_AC1018.dwg` | R2004 | 1.1 MB |
| `sample_AC1021.dwg` | `samples/sample_AC1021.dwg` | R2007 | 1.1 MB |
| `sample_AC1024.dwg` | `samples/sample_AC1024.dwg` | R2010 | 1.1 MB |
| `sample_AC1027.dwg` | `samples/sample_AC1027.dwg` | R2013 | 1.1 MB |
| `sample_AC1032.dwg` | `samples/sample_AC1032.dwg` | R2018 | 1.1 MB |
| `empty_AC1032.dwg` | `samples/sample_base/empty.dwg` | R2018 | 30 KB |

The `sample_AC10xx.dwg` files are one drawing saved by AutoCAD in each version: 21 layers (19 from
R2010), 26 lines, 8 circles, 1 arc, 21 lightweight polylines and 2 heavy polylines in model space, and
a large set of entities Cad-lite does not read (29 TEXT, 15 MULTILEADER, 11 DIMENSION, 8 HATCH and so
on). The R14 file is the same drawing with R14's upper-case layer names.

## Where the expected values come from

ACadSharp keeps an ASCII DXF export of each sample beside it (`sample_AC10xx_ascii.dxf`, 2.6 MB each,
not copied here). The values asserted in `read_test.go` (layer names and properties, entity counts,
exact coordinates, units, extents) are those of that export, read by the DXF reader. To compare
every entity of every version against the export, point `DWG_PARITY_DIR` at a checkout of the
`samples/` folder and run the parity test:

```
DWG_PARITY_DIR=/path/to/ACadSharp/samples go test ./fileio/dwg -run TestParityWithDXF -v
```
