fedora
===

Personal workstation builds for use by bootc for local deployment of fedora.
This provides a pretty simplistic way of bootstrapping a workstation
though it requires the use of an atomic spin (from fedora) the first
time so that the bundle can be built and then bootc'd into.

_This image is built and deployed locally/manually via `just`_

## util

Contains simple helpers to inspect/review/deal with images.

### oar

o(ci)-a(rchive) r(eader) will quickly/simply unpack an oci archive,
very naively, into an output directory

```
./oar <file>.ociarchive destdir/
```

to just list JSON information
```
./oar <file>.ociarchive destdir/ --mode info
```

or to list file content without fully unpacking the archive blobs
```
./oar <file>.ociarchive destdir/ --mode ls
```
