<div align="center">

<img src=".github/resources/sync-logo.png" width="auto" alt="Save Sync wordmark">
<h3 style="font-size: 25px;">
    Sync your save files to a versioned S3 Bucket
</h3>

## [Download this in Pak Store!](https://github.com/UncleJunVIP/nextui-pak-store)

![GitHub License](https://img.shields.io/github/license/UncleJunVip/nextui-s3-save-sync?style=for-the-badge)
![GitHub Release](https://img.shields.io/github/v/release/UncleJunVIP/nextui-s3-save-sync?sort=semver&style=for-the-badge)
![GitHub Repo stars](https://img.shields.io/github/stars/UncleJunVip/nextui-s3-save-sync?style=for-the-badge)
![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/UncleJunVIP/nextui-s3-save-sync/total?style=for-the-badge&label=Total%20Downloads)


</div>

---

### How do I use this Pak?

1. Create file named `config.yml` and copy this template into it.
    ```yaml
    bucket: "your-bucket-name"
    region: "us-east-1"
    
    save_directory: "/mnt/SDCARD/Saves"
    
    access_key: "YOUR_ACCESS_KEY_ID"
    secret_key: "YOUR_SECRET_ACCESS_KEY"
    log_level: "ERROR" # DEBUG | INFO | ERROR
    ```
2. On AWS, create a new S3 Bucket for your saves. **Be sure to enable versioning!**
3. Update the bucket name in the config file and set the appropriate region.
4. Make an IAM User with Read / Write permissions to this bucket.
5. Make an `Access Key` and `Secret Key` for the above user. Put these values into the config.
6. Save the config file and copy it to the Save Sync Pak directory on your device.
7. Open the Pak and Upload / Download your saves.

---

### What does this do?

It uploads your entire save directory to S3.

Since versioning is enabled it will keep copies of the saves you upload.

These versions will eventually be accessible for restoration through the Pak.

### What does this not do?

This Pak is super naive. It doesn't do anything past upload or download your save files to S3.

The versioning should keep you out of a jam, but I provide no guarantees.

It is totally possible to overwrite progress when you download from S3.

If you want something more robust, you might want to consider SyncThing. This is good enough for me.