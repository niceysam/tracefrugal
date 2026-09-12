package pack

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

func ReadReport(root string, now time.Time) (Report, error) {
	r := Report{Hours: []Hour{}, Events: []Event{}}
	// Reading a report must not create directories.
	if err := regular(filepath.Join(root, "trial.json")); err != nil {
		return r, err
	}
	b, err := os.ReadFile(filepath.Join(root, "trial.json"))
	if err != nil || json.Unmarshal(b, &r.Trial) != nil || r.Version != 1 || !r.Expires.After(r.Started) || r.Expires.Sub(r.Started) > 24*time.Hour {
		return r, errors.New("invalid trial metadata")
	}
	r.Status = "Within trial window · proxy liveness unverified"
	if !now.Before(r.Expires) {
		r.Status = "Expired · future results pass through"
	}
	if _, err := os.Lstat(filepath.Join(root, "stopped")); err == nil {
		r.Status = "Stopped · future results pass through"
	} else if !os.IsNotExist(err) {
		return r, err
	}
	r.Activation = "Waiting for a qualifying tool result"
	var firstPacked time.Time
	for i := 0; i < 24; i++ {
		r.Hours = append(r.Hours, Hour{Start: r.Started.Add(time.Duration(i) * time.Hour)})
	}
	path := filepath.Join(root, "events.jsonl")
	if err := regular(path); err == nil {
		f, err := os.Open(path)
		if err != nil {
			return r, err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var e Event
			if json.Unmarshal(scanner.Bytes(), &e) != nil || e.Time.IsZero() || e.Delivered < 0 || e.Original < 0 {
				return r, errors.New("invalid event journal")
			}
			r.Events = append(r.Events, e)
			if len(r.Events) > 100 {
				r.Events = r.Events[1:]
			}
			i := int(e.Time.Sub(r.Started) / time.Hour)
			if e.Time.Before(r.Started) {
				i = -1
			}
			switch e.Kind {
			case "packed":
				if firstPacked.IsZero() || e.Time.Before(firstPacked) {
					firstPacked = e.Time
				}
				r.Packed++
				r.Original += int64(e.Original)
				r.Delivered += int64(e.Delivered)
				if i >= 0 && i < 24 {
					r.Hours[i].Packed++
					r.Hours[i].Original += int64(e.Original)
					r.Hours[i].Delivered += int64(e.Delivered)
				}
			case "recalled":
				r.Recalls++
				r.RecallTraffic += int64(e.Delivered)
				if i >= 0 && i < 24 {
					r.Hours[i].Recalls++
					r.Hours[i].Delivered += int64(e.Delivered)
				}
			default:
				return r, errors.New("unknown journal event")
			}
		}
		if scanner.Err() != nil {
			return r, scanner.Err()
		}
	} else if !os.IsNotExist(err) {
		return r, err
	}
	if r.Packed > 0 {
		r.Activation = "Transformations recorded at the proxy output boundary; host consumption unverified"
	}
	for _, item := range []struct {
		name   string
		target *int
	}{{"before-rating.json", &r.BeforeRating}, {"after-rating.json", &r.AfterRating}} {
		path := filepath.Join(root, item.name)
		if err := regular(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return r, err
		}
		f, err := os.Open(path)
		if err != nil {
			return r, err
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			var rating struct {
				Rating int       `json:"rating"`
				Time   time.Time `json:"time"`
			}
			if json.Unmarshal(s.Bytes(), &rating) != nil || rating.Time.IsZero() || rating.Rating < 1 || rating.Rating > 5 {
				f.Close()
				return r, errors.New("invalid rating history")
			}
			// A baseline that raced with the first transformation must not be
			// presented as a pre-change assessment.
			if item.name != "before-rating.json" || firstPacked.IsZero() || rating.Time.Before(firstPacked) {
				*item.target = rating.Rating
			}
		}
		err = s.Err()
		f.Close()
		if err != nil {
			return r, err
		}
	}
	return r, nil
}
