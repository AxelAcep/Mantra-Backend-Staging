package models

import "time"

type Barang struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	NoBarang  string    `gorm:"not null;uniqueIndex" json:"noBarang"`
	Deskripsi string    `gorm:"not null" json:"deskripsi"`
	Satuan    string    `gorm:"not null" json:"satuan"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (Barang) TableName() string { return "Barang" }
