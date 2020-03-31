[![Build Status](https://travis-ci.com/iamjinlei/gitfs.svg?branch=master)](https://travis-ci.com/iamjinlei/gitfs)

# gitfs 
GitFs provides you an alternative persistent storage solution for keeping metadata. This is ideal for quick prototyping. GitFs has a choice of memfs or OS file system as a local storage, which is backed by remote git repo. When syncing local data, it has an option to wipe out git history to boost sync efficiency.

```golang
c := gitfs.NewConfig().SetUrl(repoUrl).UseMemFs()
fs, _ := gitfs.New(context.TODO(), c)

f, _ := fs.Create("test.txt")
f.Write([]byte("test"))
f.Close()

fs.Sync(true)
```

Limitation: src-d has no support for git merge yet. It could fail to sync if remote repo is diverged.

# example
```bash
go run example/run.go
```
