alpine-image-builder
===

Wrapper around Alpine Linux
[mkimage.sh](https://gitlab.alpinelinux.org/alpine/aports/-/tree/master/scripts?ref_type=heads)
tooling for creating ISOs/images. Much of the configuration/documentation for Alpine
Linux ISO/image generation can be abstracted away, which this quick wrapper aims to do
to generate personalized variants.

## usage

To use
1. Fork 
2. Set GitHub secrets: `ALPINE_IMAGE_RSA_KEY` and `ALPINE_IMAGE_RSA_KEY_PUBLIC` (from `abuild-keygen`)
3. Set GitHub variable: `ALPINE_IMAGE_KEY_NAME` to the name of the RSA key (from
   abuild configuration, this should mirror a local system for debugging)
4. Update `builder.toml` (specify additional `commands` to inject additional
   key/value pairs into the template)
5. Run a build
6. Files are archived at the end of the action

(can also be run locally, on Alpine and look in `build/`)
