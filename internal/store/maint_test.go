package store

import (
	"testing"
	"time"
)

func TestInMaintenance(t *testing.T) {
	// jueves 2026-08-27 03:00 (hora local)
	thu := time.Date(2026, 8, 27, 3, 0, 0, 0, time.Local)

	// ventana normal: jueves 00:00-04:00
	m := Monitor{MaintEnabled: true, MaintWeekday: 4, MaintStart: "00:00", MaintEnd: "04:00"}
	if !m.InMaintenance(thu) {
		t.Error("jueves 03:00 debería estar en mantenimiento")
	}
	if m.InMaintenance(thu.Add(3 * time.Hour)) { // 06:00
		t.Error("jueves 06:00 debería estar fuera de mantenimiento")
	}

	// cruce de medianoche: domingo 23:00 -> lunes 01:00
	m2 := Monitor{MaintEnabled: true, MaintWeekday: 0, MaintStart: "23:00", MaintEnd: "01:00"}
	sun := time.Date(2026, 8, 23, 23, 30, 0, 0, time.Local) // domingo
	mon := time.Date(2026, 8, 24, 0, 30, 0, 0, time.Local)  // lunes
	if !m2.InMaintenance(sun) {
		t.Error("domingo 23:30 debería estar en mantenimiento")
	}
	if !m2.InMaintenance(mon) {
		t.Error("lunes 00:30 debería estar en mantenimiento (cruce de medianoche)")
	}
	if m2.InMaintenance(mon.Add(2 * time.Hour)) { // lunes 02:30
		t.Error("lunes 02:30 debería estar fuera de mantenimiento")
	}

	// desactivado y ventana vacía
	m3 := Monitor{MaintEnabled: false}
	if m3.InMaintenance(thu) {
		t.Error("desactivado nunca está en mantenimiento")
	}
	m4 := Monitor{MaintEnabled: true, MaintWeekday: 4, MaintStart: "10:00", MaintEnd: "10:00"}
	if m4.InMaintenance(thu) {
		t.Error("ventana con inicio=fin no debe activarse")
	}
}
