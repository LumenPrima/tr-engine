package testutil

import (
	"testing"
	"time"
)

// TestFixtures contains IDs and values for the seeded test data
type TestFixtures struct {
	// Instances
	Instance1ID int

	// Systems
	MetroSystemID  int // sysid: 348
	CountySystemID int // sysid: 15a

	// Talkgroup natural keys (sysid, tgid)
	MetroPDMainSYSID  string // "348"
	MetroPDMainTGID   int    // 9178
	CountyPDMainSYSID string // "15a"
	CountyPDMainTGID  int    // 9178
	MetroFireSYSID    string // "348"
	MetroFireTGID     int    // 9200
	MetroEMSSYSID     string // "348"
	MetroEMSTGID      int    // 9300

	// Unit natural keys (sysid, unit_id)
	MetroUnit1SYSID   string // "348"
	MetroUnit1UnitID  int64  // 1001234
	MetroUnit2SYSID   string // "348"
	MetroUnit2UnitID  int64  // 1001235
	CountyUnit1SYSID  string // "15a"
	CountyUnit1UnitID int64  // 1001234

	// Calls
	Call1ID int64 // Metro PD Main, with audio
	Call2ID int64 // Metro PD Main, with audio
	Call3ID int64 // Metro Fire, with audio
	Call4ID int64 // County PD Main, with audio
	Call5ID int64 // Metro PD Main, encrypted (no audio)

	// Base time for test data
	BaseTime time.Time
}

// SeedTestData populates the database with known test data for integration tests.
// Returns a TestFixtures struct with IDs for reference in assertions.
func SeedTestData(t testing.TB, db *TestDB) *TestFixtures {
	t.Helper()

	f := &TestFixtures{
		BaseTime: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	// Instance
	db.MustExec(t, `INSERT INTO instances (id, instance_id, instance_key) VALUES (1, 'test-instance', 'test-key')`)
	f.Instance1ID = 1

	// Systems - two different P25 networks with different sysids
	db.MustExec(t, `INSERT INTO systems (id, instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id)
		VALUES (1, 1, 1, 'metro', 'p25', '348', 'BEE00', '293', 1, 1)`)
	db.MustExec(t, `INSERT INTO systems (id, instance_id, sys_num, short_name, system_type, sysid, wacn, nac, rfss, site_id)
		VALUES (2, 1, 2, 'county', 'p25', '15a', 'BEE00', '294', 1, 2)`)
	f.MetroSystemID = 1
	f.CountySystemID = 2

	// Talkgroups - note: same tgid (9178) exists in both systems
	// Natural key is (sysid, tgid), no database ID
	db.MustExec(t, `INSERT INTO talkgroups (sysid, tgid, alpha_tag, description, tg_group, tag, priority, mode)
		VALUES ('348', 9178, 'Metro PD Main', 'Metro Police Dispatch', 'Law Enforcement', 'Law Dispatch', 1, 'D')`)
	db.MustExec(t, `INSERT INTO talkgroups (sysid, tgid, alpha_tag, description, tg_group, tag, priority, mode)
		VALUES ('15a', 9178, 'County PD Main', 'County Police Dispatch', 'Law Enforcement', 'Law Dispatch', 1, 'D')`)
	db.MustExec(t, `INSERT INTO talkgroups (sysid, tgid, alpha_tag, description, tg_group, tag, priority, mode)
		VALUES ('348', 9200, 'Metro Fire', 'Metro Fire Dispatch', 'Fire', 'Fire Dispatch', 2, 'D')`)
	db.MustExec(t, `INSERT INTO talkgroups (sysid, tgid, alpha_tag, description, tg_group, tag, priority, mode)
		VALUES ('348', 9300, 'Metro EMS', 'Metro EMS Operations', 'EMS', 'EMS Dispatch', 3, 'D')`)
	f.MetroPDMainSYSID = "348"
	f.MetroPDMainTGID = 9178
	f.CountyPDMainSYSID = "15a"
	f.CountyPDMainTGID = 9178
	f.MetroFireSYSID = "348"
	f.MetroFireTGID = 9200
	f.MetroEMSSYSID = "348"
	f.MetroEMSTGID = 9300

	// Link talkgroups to systems
	db.MustExec(t, `INSERT INTO talkgroup_sites (sysid, tgid, system_id) VALUES
		('348', 9178, 1), ('15a', 9178, 2), ('348', 9200, 1), ('348', 9300, 1)`)

	// Units - note: same unit_id (1001234) exists in both systems
	// Natural key is (sysid, unit_id), no database ID
	db.MustExec(t, `INSERT INTO units (sysid, unit_id, alpha_tag, alpha_tag_source)
		VALUES ('348', 1001234, 'Metro Car 123', 'radioreference')`)
	db.MustExec(t, `INSERT INTO units (sysid, unit_id, alpha_tag, alpha_tag_source)
		VALUES ('348', 1001235, 'Metro Car 124', 'radioreference')`)
	db.MustExec(t, `INSERT INTO units (sysid, unit_id, alpha_tag, alpha_tag_source)
		VALUES ('15a', 1001234, 'County Unit 1', 'manual')`)
	f.MetroUnit1SYSID = "348"
	f.MetroUnit1UnitID = 1001234
	f.MetroUnit2SYSID = "348"
	f.MetroUnit2UnitID = 1001235
	f.CountyUnit1SYSID = "15a"
	f.CountyUnit1UnitID = 1001234

	// Link units to systems
	db.MustExec(t, `INSERT INTO unit_sites (sysid, rid, system_id) VALUES
		('348', 1001234, 1), ('348', 1001235, 1), ('15a', 1001234, 2)`)

	// Calls - mix of systems and talkgroups
	// Call 1: Metro PD Main, 30 seconds ago, with audio
	db.MustExec(t, `INSERT INTO calls (id, instance_id, system_id, tg_sysid, tgid, tr_call_id, call_num,
		start_time, stop_time, duration, audio_path, audio_size, encrypted, emergency,
		call_state, mon_state, phase2_tdma, tdma_slot, conventional, analog, audio_type,
		freq, freq_error, error_count, spike_count, signal_db, noise_db)
		VALUES (1, 1, 1, '348', 9178, '1705312200_850387500_9178', 1001, $1, $2, 15.5,
		'metro/2026/01/15/9178-1705312200.m4a', 45000, false, false,
		0, 0, false, 0, false, false, 'digital',
		850387500, 0, 0, 0, -45.0, -90.0)`,
		f.BaseTime.Add(-30*time.Second), f.BaseTime.Add(-15*time.Second))

	// Call 2: Metro PD Main, 60 seconds ago, with audio
	db.MustExec(t, `INSERT INTO calls (id, instance_id, system_id, tg_sysid, tgid, tr_call_id, call_num,
		start_time, stop_time, duration, audio_path, audio_size, encrypted, emergency,
		call_state, mon_state, phase2_tdma, tdma_slot, conventional, analog, audio_type,
		freq, freq_error, error_count, spike_count, signal_db, noise_db)
		VALUES (2, 1, 1, '348', 9178, '1705312100_850387500_9178', 1002, $1, $2, 22.3,
		'metro/2026/01/15/9178-1705312100.m4a', 67000, false, false,
		0, 0, false, 0, false, false, 'digital',
		850387500, 0, 0, 0, -42.0, -88.0)`,
		f.BaseTime.Add(-60*time.Second), f.BaseTime.Add(-38*time.Second))

	// Call 3: Metro Fire, 90 seconds ago, with audio, emergency
	db.MustExec(t, `INSERT INTO calls (id, instance_id, system_id, tg_sysid, tgid, tr_call_id, call_num,
		start_time, stop_time, duration, audio_path, audio_size, encrypted, emergency,
		call_state, mon_state, phase2_tdma, tdma_slot, conventional, analog, audio_type,
		freq, freq_error, error_count, spike_count, signal_db, noise_db)
		VALUES (3, 1, 1, '348', 9200, '1705312000_851000000_9200', 1003, $1, $2, 45.0,
		'metro/2026/01/15/9200-1705312000.m4a', 135000, false, true,
		0, 0, false, 0, false, false, 'digital',
		851000000, 0, 0, 0, -48.0, -92.0)`,
		f.BaseTime.Add(-90*time.Second), f.BaseTime.Add(-45*time.Second))

	// Call 4: County PD Main, 120 seconds ago, with audio
	db.MustExec(t, `INSERT INTO calls (id, instance_id, system_id, tg_sysid, tgid, tr_call_id, call_num,
		start_time, stop_time, duration, audio_path, audio_size, encrypted, emergency,
		call_state, mon_state, phase2_tdma, tdma_slot, conventional, analog, audio_type,
		freq, freq_error, error_count, spike_count, signal_db, noise_db)
		VALUES (4, 1, 2, '15a', 9178, '1705311900_852000000_9178', 1004, $1, $2, 18.7,
		'county/2026/01/15/9178-1705311900.m4a', 56000, false, false,
		0, 0, false, 0, false, false, 'digital',
		852000000, 0, 0, 0, -50.0, -95.0)`,
		f.BaseTime.Add(-120*time.Second), f.BaseTime.Add(-101*time.Second))

	// Call 5: Metro PD Main, encrypted (no audio path)
	db.MustExec(t, `INSERT INTO calls (id, instance_id, system_id, tg_sysid, tgid, tr_call_id, call_num,
		start_time, stop_time, duration, encrypted, emergency,
		call_state, mon_state, phase2_tdma, tdma_slot, conventional, analog, audio_type,
		freq, freq_error, error_count, spike_count, signal_db, noise_db)
		VALUES (5, 1, 1, '348', 9178, '1705311800_850387500_9178', 1005, $1, $2, 10.0, true, false,
		0, 0, false, 0, false, false, 'digital',
		850387500, 0, 0, 0, -55.0, -98.0)`,
		f.BaseTime.Add(-150*time.Second), f.BaseTime.Add(-140*time.Second))

	f.Call1ID = 1
	f.Call2ID = 2
	f.Call3ID = 3
	f.Call4ID = 4
	f.Call5ID = 5

	// Transmissions for Call 1 - use unit_sysid instead of unit_id
	db.MustExec(t, `INSERT INTO transmissions (id, call_id, unit_sysid, unit_rid, start_time, duration, position, emergency, error_count, spike_count)
		VALUES (1, 1, '348', 1001234, $1, 5.0, 0.0, false, 0, 0)`, f.BaseTime.Add(-30*time.Second))
	db.MustExec(t, `INSERT INTO transmissions (id, call_id, unit_sysid, unit_rid, start_time, duration, position, emergency, error_count, spike_count)
		VALUES (2, 1, '348', 1001235, $1, 4.5, 5.5, false, 0, 0)`, f.BaseTime.Add(-24*time.Second))
	db.MustExec(t, `INSERT INTO transmissions (id, call_id, unit_sysid, unit_rid, start_time, duration, position, emergency, error_count, spike_count)
		VALUES (3, 1, '348', 1001234, $1, 5.0, 10.5, false, 0, 0)`, f.BaseTime.Add(-19*time.Second))

	// Unit events - use unit_sysid and tg_sysid instead of unit_id and talkgroup_id
	db.MustExec(t, `INSERT INTO unit_events (id, instance_id, system_id, unit_sysid, unit_rid, event_type, tg_sysid, tgid, time)
		VALUES (1, 1, 1, '348', 1001234, 'call', '348', 9178, $1)`, f.BaseTime.Add(-30*time.Second))
	db.MustExec(t, `INSERT INTO unit_events (id, instance_id, system_id, unit_sysid, unit_rid, event_type, tg_sysid, tgid, time)
		VALUES (2, 1, 1, '348', 1001235, 'call', '348', 9178, $1)`, f.BaseTime.Add(-24*time.Second))
	db.MustExec(t, `INSERT INTO unit_events (id, instance_id, system_id, unit_sysid, unit_rid, event_type, tg_sysid, tgid, time)
		VALUES (3, 1, 1, '348', 1001234, 'join', '348', 9178, $1)`, f.BaseTime.Add(-60*time.Second))

	// System rates
	db.MustExec(t, `INSERT INTO system_rates (id, system_id, time, decode_rate, control_channel)
		VALUES (1, 1, $1, 98.5, 851012500)`, f.BaseTime.Add(-10*time.Second))
	db.MustExec(t, `INSERT INTO system_rates (id, system_id, time, decode_rate, control_channel)
		VALUES (2, 2, $1, 95.2, 852025000)`, f.BaseTime.Add(-10*time.Second))

	// Reset sequences to avoid conflicts with explicitly-inserted IDs
	// Set them to 100 to leave room for fixture data
	db.MustExec(t, `SELECT setval('instances_id_seq', 100)`)
	db.MustExec(t, `SELECT setval('systems_id_seq', 100)`)
	db.MustExec(t, `SELECT setval('calls_id_seq', 100)`)
	db.MustExec(t, `SELECT setval('transmissions_id_seq', 100)`)
	db.MustExec(t, `SELECT setval('unit_events_id_seq', 100)`)
	db.MustExec(t, `SELECT setval('system_rates_id_seq', 100)`)

	return f
}
