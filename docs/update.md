# To update the AV run the following:

```bash
$ docker run --name=avast malice/avast update
```

## Then to use the updated AVG container:

```bash
$ docker commit avast malice/avast:updated
$ docker rm avast # clean up updated container
$ docker run --rm malice/avast:updated EICAR
```
