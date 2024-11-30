alpine
===

Wrapper around Alpine Linux
[mkimage.sh](https://gitlab.alpinelinux.org/alpine/aports/-/tree/master/scripts?ref_type=heads)
tooling for creating ISOs/images. Much of the configuration/documentation for Alpine
Linux ISO/image generation can be abstracted away, which this quick wrapper aims to do
to generate personalized variants.

## usage

To use
1. Set GitHub secrets: `IMAGE_RSA_KEY` and `IMAGE_RSA_KEY_PUBLIC` (from `abuild-keygen`)
2. Set GitHub variable: `IMAGE_KEY_NAME` to the name of the RSA key (from
   abuild configuration, this should mirror a local system for debugging)
3. Update `builder.toml` (specify additional `commands` to inject additional
   key/value pairs into the template)
4. Run a build
5. Files are archived at the end of the action

(can also be run locally, on Alpine, look in `build/`)
