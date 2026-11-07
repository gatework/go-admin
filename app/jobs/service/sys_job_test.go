package service

import (
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg/cronjob"
	coreservice "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"go-admin/app/jobs/models"
	"go-admin/common/dto"
)

func TestRepeatedStartThenStopLeavesNoEntries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.SysJob{}); err != nil {
		t.Fatal(err)
	}
	job := models.SysJob{JobType: 1, Status: 2, CronExpression: "0 * * * *"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	c := cronjob.NewWithSeconds()
	s := SysJob{Service: coreservice.Service{Orm: db}, Cron: c}
	for range 2 {
		if err := s.StartJob(&dto.GeneralGetDto{Id: job.JobId}); err != nil {
			t.Fatal(err)
		}
	}
	var starts sync.WaitGroup
	for range 12 {
		starts.Add(1)
		go func() {
			defer starts.Done()
			if err := s.StartJob(&dto.GeneralGetDto{Id: job.JobId}); err != nil {
				t.Error(err)
			}
		}()
	}
	starts.Wait()
	if got := len(c.Entries()); got != 1 {
		t.Errorf("repeated start registered %d entries, want 1", got)
	}
	if err := s.RemoveJob(&dto.GeneralDelDto{Id: job.JobId}); err != nil {
		t.Fatal(err)
	}
	if got := len(c.Entries()); got != 0 {
		t.Errorf("stop left %d entries", got)
	}
	var saved models.SysJob
	if err := db.First(&saved, job.JobId).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != 1 {
		t.Errorf("stop not durable: status=%d", saved.Status)
	}
}

type blockedSchedule struct {
	started chan struct{}
	release chan struct{}
}

func (s blockedSchedule) Next(now time.Time) time.Time {
	close(s.started)
	<-s.release
	return now.Add(time.Hour)
}

func TestRemoveJob(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "success"
		if blocked {
			name = "timeout"
		}
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&models.SysJob{}); err != nil {
				t.Fatal(err)
			}

			c := cron.New()
			schedule := blockedSchedule{make(chan struct{}), make(chan struct{})}
			entryID := c.Schedule(schedule, cron.FuncJob(func() {}))
			if blocked {
				c.Start()
				t.Cleanup(func() {
					close(schedule.release)
					<-c.Stop().Done()
				})
				select {
				case <-schedule.started:
				case <-time.After(5 * time.Second):
					t.Fatal("scheduler did not start")
				}
			}
			job := models.SysJob{EntryId: int(entryID)}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			s := SysJob{Service: coreservice.Service{Orm: db}, Cron: c}
			err = s.RemoveJob(&dto.GeneralDelDto{Id: job.JobId})
			if blocked {
				if err == nil || err.Error() != "操作超时！" {
					t.Errorf("RemoveJob error = %v, want timeout error", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			var saved models.SysJob
			if err := db.First(&saved, job.JobId).Error; err != nil {
				t.Fatal(err)
			}
			wantEntryID := 0
			if blocked {
				wantEntryID = int(entryID)
			}
			if saved.EntryId != wantEntryID {
				t.Errorf("entry_id = %d, want %d", saved.EntryId, wantEntryID)
			}
		})
	}
}
