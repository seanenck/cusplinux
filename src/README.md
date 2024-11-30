image-build
===

Tooling to support building custom linux images

## alpine

definitions to develop/build a personal alpine iso/image for development

## fedora

workstation builds for atomic fedora variants

## src (tools)

### alpine-image-builder

used by the `alpine/` content to build ISO/rootfs/etc. images. Embeds
`obu` into the image for usage

###  obu

An overlay backup utility to support using overlay backup as a sort of `lbu`
(alpine tool) system to be able to naively run in memory with a backing store
that can be committed to for file modifications

configurations are in the `obu/` directory
