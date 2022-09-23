package logger

import (
	"fmt"
	"os"
)

func checkNotExist(src string) bool {
	_, err := os.Stat(src)
	return os.IsNotExist(err)
}

func checkPermission(src string) bool {
	_, err := os.Stat(src)
	return os.IsPermission(err)
}

func mkDir(src string) error {
	err := os.MkdirAll(src, os.ModeAppend)
	if err != nil {
		return err
	}

	return nil
}

func isNotExistMkDir(src string) error {
	if notExist := checkNotExist(src); notExist == true {
		if err := mkDir(src); err != nil {
			return err
		}
	}
	return nil
}

func mustOpen(fileName, dir string) (*os.File, error) {
	perm := checkPermission(fileName)
	if perm == true {
		return nil, fmt.Errorf("permission denied dir: %s", dir)
	}

	err := isNotExistMkDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error during make dir %s, err: %s", dir, err)
	}

	fp, err := os.OpenFile(dir+string(os.PathSeparator)+fileName,
		os.O_APPEND|os.O_CREATE|os.O_RDWR,
		0644)
	if err != nil {
		return nil, fmt.Errorf("fail to open file, err: %s", err)
	}

	return fp, nil
}
