package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"io/ioutil"

	//"github.com/mitchellh/goamz/aws"
	//"github.com/mitchellh/goamz/s3"
	//awscommon "github.com/mitchellh/packer/builder/amazon/common"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/helper/config"
	"github.com/mitchellh/packer/template/interpolate"
)

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	
	// Fields from config file
	//Region       string `mapstructure:"region"`
	//Bucket       string `mapstructure:"bucket"`
	ManifestPath string `mapstructure:"manifest"`
	BoxName      string `mapstructure:"box_name"`
	BoxDir       string `mapstructure:"box_dir"`
	Version      string `mapstructure:"version"`
	Path		 string `mapstructure:"path"`
	//ACL          s3.ACL `mapstructure:"acl"`

	
	//awscommon.AccessConfig `mapstructure:",squash"`
	//tpl                    *packer.ConfigTemplate
	
	ctx interpolate.Context
}

type PostProcessor struct {
	config Config
	//s3     *s3.Bucket
}

func (p *PostProcessor) Configure(raws ...interface{}) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
	}, raws...)
	if err != nil {
		return err
	}

	// p.config.tpl, err = packer.NewConfigTemplate()
	// if err != nil {
	// 	return err
	// }
	// p.config.tpl.UserVars = p.config.PackerUserVars

	errs := &packer.MultiError{}

	// errs = packer.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(p.config.tpl)...)

	// required configuration
	templates := map[string]*string{
		"path":   &p.config.Path,
		//"bucket":   &p.config.Bucket,
		"manifest": &p.config.ManifestPath,
		"box_name": &p.config.BoxName,
		"box_dir":  &p.config.BoxDir,
		"version":  &p.config.Version,
	}

	for key, ptr := range templates {
		if *ptr == "" {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("vagrant-path %s must be set", key))
		}
	}

	// Template process
	// for key, ptr := range templates {
	// 	*ptr, err = p.config.tpl.Process(*ptr, nil)
	// 	if err != nil {
	// 		errs = packer.MultiErrorAppend(errs, fmt.Errorf("Error processing %s: %s", key, err))
	// 	}
	// }

	// setup the s3 bucket
	// auth, err := aws.GetAuth(p.config.AccessConfig.AccessKey, p.config.AccessConfig.SecretKey)
	// if err != nil {
	// 	errs = packer.MultiErrorAppend(errs, err)
	// }

	// determine region
	// region, valid := aws.Regions[p.config.Region]
	// if valid {
	// 	p.s3 = s3.New(auth, region).Bucket(p.config.Bucket)
	// } else {
	// 	errs = packer.MultiErrorAppend(errs, fmt.Errorf("Invalid region specified: %s", p.config.Region))
	// }

	// if p.config.ACL == "" {
	// 	p.config.ACL = "public-read"
	// }

	if len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

func (p *PostProcessor) PostProcess(ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, error) {
	// Only accept input from the vagrant post-processor
	if artifact.BuilderId() != "mitchellh.post-processor.vagrant" {
		return nil, false, fmt.Errorf("Unknown artifact type, requires box from vagrant post-processor: %s", artifact.BuilderId())
	}
	
	// Assume there is only one .box file to upload
	box := artifact.Files()[0]
	if !strings.HasSuffix(box, ".box") {
		return nil, false, fmt.Errorf("Unknown files in artifact from vagrant post-processor: %s", artifact.Files())
	}

	provider := providerFromBuilderName(artifact.Id())
	ui.Say(fmt.Sprintf("Preparing to copy box for '%s' provider to path '%s'", provider, p.config.Path))

	// open the box so we can copy to path and calculate checksum for the manifest
	file, err := os.Open(box)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	// get the file's size
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	size := info.Size()
	ui.Message(fmt.Sprintf("Box to copy: %s (%d bytes)", box, size))

	// get the latest manifest so we can add to it
	ui.Message("Fetching latest manifest")
	manifest, err := p.getManifest()
	if err != nil {
		return nil, false, err
	}

	// generate the path to copy the box to the path
	boxPath := fmt.Sprintf("%s/%s/%s", p.config.BoxDir, p.config.Version, path.Base(box))

	ui.Message("Generating checksum")
	checksum, err := sum256(file)
	if err != nil {
		return nil, false, err
	}
	ui.Message(fmt.Sprintf("Checksum is %s", checksum))

	ui.Message(fmt.Sprintf("Adding %s %s box to manifest", provider, p.config.Version))
	if err := manifest.add(p.config.Version, &Provider{
		Name:         provider,
		Url:          p.config.Path,
		ChecksumType: "sha256",
		Checksum:     checksum,
	}); err != nil {
		return nil, false, err
	}

	// upload the box to S3 (rewinding as we already read the file to generate the checksum)
	ui.Message(fmt.Sprintf("Copying box to path: %s", boxPath))
	if _, err := file.Seek(0, 0); err != nil {
		return nil, false, err
	}
	// if size > 100*1024*1024 {
	// 	ui.Message("File size > 100MB. Initiating multipart upload")
	// 	multi, err := p.s3.Multi(boxPath, "application/octet-stream", p.config.ACL)
	// 	if err != nil {
	// 		return nil, false, err
	// 	}

	// 	parts, err := multi.PutAll(file, 5*1024*1024)
	// 	if err != nil {
	// 		return nil, false, err
	// 	}

	// 	if err := multi.Complete(parts); err != nil {
	// 		return nil, false, err
	// 	}
	// } else {
	// 	if err := p.s3.PutReader(boxPath, file, size, "application/octet-stream", p.config.ACL); err != nil {
	// 		return nil, false, err
	// 	}
	// }
	
	in, err := os.Open(box)
    if err != nil { 
		return nil, false, err
	}
    defer in.Close()
	
    out, err := os.Create(boxPath)
    if err != nil {
		return nil, false, err
	}
    defer out.Close()
	
    _, err = io.Copy(out, in)
    cerr := out.Close()
    if err != nil { 
		return nil, false, err
	}
    return nil, false, cerr

	ui.Message(fmt.Sprintf("Uploading the manifest: %s", p.config.ManifestPath))
	if err := p.putManifest(manifest); err != nil {
		return nil, false, err
	}

	

	//return &Artifact{p.s3.URL(p.config.ManifestPath)}, true, nil
	return &Artifact{p.config.ManifestPath}, true, nil
}

func (p *PostProcessor) getManifest() (*Manifest, error) {	
	if _, err := os.Stat(p.config.ManifestPath); err == nil {
		//body, err := ioutil.ReadFile(p.config.ManifestPath)
		file, err := os.Open(p.config.ManifestPath)
		if err != nil {
			return nil, err
		}
		manifest := &Manifest{}
		if err := json.NewDecoder(file).Decode(manifest); err != nil {
			return nil, err
		}
		return manifest, nil
	} else {
		return &Manifest{Name: p.config.BoxName}, nil
	}
}

func (p *PostProcessor) putManifest(manifest *Manifest) error {
	// var buf bytes.Buffer
	// if err := json.NewEncoder(&buf).Encode(manifest); err != nil {
	// 	return err
	// }
	// if err := p.s3.Put(p.config.ManifestPath, buf.Bytes(), "application/json", p.config.ACL); err != nil {
	// 	return err
	// }
	// return nil
	
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(manifest); err != nil {
		return err
	}
	
	err := ioutil.WriteFile(p.config.ManifestPath, buf.Bytes(), 0644)
	if err != nil {
		return err
	}
	
	return nil
	
	// in, err := os.Open(manifest)
    // if err != nil { return err }
    // defer in.Close()
    // out, err := os.Create(p.config.ManifestPath)
    // if err != nil { return err }
    // defer out.Close()
    // _, err = io.Copy(out, in)
    // cerr := out.Close()
    // if err != nil { return err }
    // return cerr
}

// calculates a sha256 checksum of the file
func sum256(file *os.File) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// converts a packer builder name to the corresponding vagrant provider
func providerFromBuilderName(name string) string {
	switch name {
	case "aws":
		return "aws"
	case "digitalocean":
		return "digitalocean"
	case "virtualbox":
		return "virtualbox"
	case "vmware":
		return "vmware_desktop"
	case "parallels":
		return "parallels"
	default:
		return name
	}
}
