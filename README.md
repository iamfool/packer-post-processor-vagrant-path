Packer Vagrant path post-processor
================================

Copies built Vagrant boxes to a path and manages a manifest file for versioned boxes.

Installation
------------
1. [Install git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
2. [Install go](https://golang.org/dl/)
3. Ensure you have set the [GOPATH](https://golang.org/doc/code.html#GOPATH)
4. Open a terminal window and run the following command:
 
```
go get github.com/Imagus/packer-post-processor-vagrant-path
```
5. Copy the plugin to one of the directories [outlined here](https://www.packer.io/docs/extend/plugins.html).
  * On a *nix system: `cp $GOPATH/bin/packer-post-processor-vagrant-s3 <your_target_directory>`
  * On Windows: `copy %GOPATH%/bin/packer-post-processor-vagrant-s3.exe <your_target_directory>`
    
Usage
-----

Add the post-processor to your packer template **after** the `vagrant` post-processor:

```
{
  "variables": {
    "version": "1.0.0"
  },
  "builders": [ ... ],
  "provisioners": [ ... ],
  "post-processors": [
    [
      {
        "type": "vagrant"
        ...
      },
      {
        "type":     "vagrant-s3",
        "path":   "\\\\127.0.0.1/",
        "manifest": "vagrant/manifest.json",
        "box_name": "my-cool-project",
        "box_dir":  "vagrant/boxes",
        "version":  "{{ user `version` }}"
      }
    ]
  ]
}
```
**NOTE:** 
1. The post-processors must be a **nested array**, i.e., a Packer sequence definition, so that they run in order. See the [Packer template documentation](http://www.packer.io/docs/templates/post-processors.html) for more information. Not doing this will cause the Packer build to fail due to `s3-vagrant` not receiving the correct Artifact type.
2. If you set a value for `only` on the `vagrant` post-processor, you must set that same value on the `vagrant-s3` post-processor.  

The above will result in the following object created in the specified path, a manifest:

```
<your/path>/<manifest>
```
and a box:

```
<your/path>/<box-dir>/<box-name>/<version>/<box-name>.box
  
```


Configuration
-------------

All configuration properties are **required**, except where noted.
### path

The local path here you want to copy the box and the manifest.

### manifest

The path to the manifest file in your bucket. If you don't have a manifest, don't worry, one will be created.  **We recommend that you name you manifest the same as your box.**

This controls what users of your box will set `vm.config.box_url` to in their `Vagrantfile` (e.g. `\\127.0.0.1/vagrant/manifest.json`).

### box_name

The name of your box.

This is what users of your box will set `vm.config.box` to in their `Vagrantfile`.

### box_dir

The path to a directory in your bucket to store boxes in (e.g. `vagrant/boxes`).

### version

The version of the box you are uploading. The box will be uploaded to a S3 directory path that includes the version number (e.g. `vagrant/boxes/<version>`).

Only one box can be uploaded per provider for a given version. If you are building an updated box, you should bump this version, meaning users of your box will be made aware of the new version.
