package storage

import "errors"

var (
	ErrIncidentNotFound = errors.New("异常不存在")
	ErrTaskNotFound     = errors.New("任务不存在")
)
