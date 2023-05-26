package main

type DiskStatus struct {
	total_bytes      int64
	free_bytes       int64
	bytes_per_second int64
}

type DiskWatcher interface {
	Status() (DiskStatus, error)
}
