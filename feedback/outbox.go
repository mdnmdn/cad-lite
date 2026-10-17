package feedback

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Outbox stores reports in a private directory, separately from drawings and agent history.
type Outbox struct {
	// Dir is the private report directory.
	Dir string
}

// Save atomically persists a report without service credentials.
func (o *Outbox) Save(r Report) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if o == nil || o.Dir == "" {
		return errors.New("Feedback outbox is unavailable.")
	}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return errors.New("Cannot create the feedback outbox.")
	}
	if err := os.Chmod(o.Dir, 0o700); err != nil {
		return errors.New("Cannot secure the feedback outbox.")
	}
	data, _ := json.Marshal(r)
	f, err := os.CreateTemp(o.Dir, ".report-*")
	if err != nil {
		return errors.New("Cannot write the feedback outbox.")
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(o.Dir, r.ID+".json"))
	}
	if err != nil {
		return errors.New("Cannot save the feedback report.")
	}
	return nil
}

// Remove deletes a delivered or explicitly discarded report; a missing report is harmless.
func (o *Outbox) Remove(id string) error {
	if o == nil || o.Dir == "" || !validID(id) {
		return errors.New("Invalid feedback outbox entry.")
	}
	err := os.Remove(filepath.Join(o.Dir, id+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// List loads valid pending reports in ID order, skipping corrupt and non-regular entries.
func (o *Outbox) List() ([]Report, error) {
	if o == nil || o.Dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(o.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("Cannot read pending feedback.")
	}
	var reports []Report
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !validID(id) || !entry.Type().IsRegular() {
			continue
		}
		stat, err := entry.Info()
		if err != nil || stat.Size() > 15000000 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(o.Dir, entry.Name()))
		if err != nil {
			continue
		}
		var r Report
		if json.Unmarshal(data, &r) == nil && r.ID == id && r.Validate() == nil {
			reports = append(reports, r)
		}
	}
	return reports, nil
}
