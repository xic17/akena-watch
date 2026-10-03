package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// TestCreateFirstUser comprueba que el primer administrador solo se crea una
// vez y que la segunda tentativa recibe ErrAlreadyInstalled.
func TestCreateFirstUser(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "first.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateFirstUser("akena", "hash", RoleAdmin, "akena@ejemplo.com", "123456")
	if err != nil {
		t.Fatalf("CreateFirstUser: %v", err)
	}
	if u.ID == 0 || u.Role != RoleAdmin || u.Email != "akena@ejemplo.com" {
		t.Fatalf("usuario creado inesperado: %+v", u)
	}

	if _, err := s.CreateFirstUser("otro", "hash", RoleAdmin, "", ""); !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("esperaba ErrAlreadyInstalled, got %v", err)
	}
	n, err := s.CountUsers()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("usuarios = %d, esperado 1", n)
	}
}

// TestCreateFirstUserConcurrente reproduce la carrera de la instalación: solo
// una de las peticiones simultáneas puede quedar como propietaria del sistema.
func TestCreateFirstUserConcurrente(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "carrera.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const intentos = 8
	var creados int32
	var wg sync.WaitGroup
	for i := 0; i < intentos; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.CreateFirstUser(fmt.Sprintf("admin%d", i), "hash", RoleAdmin, "", "")
			switch {
			case err == nil:
				atomic.AddInt32(&creados, 1)
			case errors.Is(err, ErrAlreadyInstalled):
				// esperado: otra petición llegó antes
			default:
				t.Errorf("error inesperado: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if creados != 1 {
		t.Fatalf("administradores creados = %d, esperado 1", creados)
	}
	n, err := s.CountUsers()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("usuarios = %d, esperado 1", n)
	}
}
