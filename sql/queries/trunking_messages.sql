-- name: InsertTrunkingMessages :copyfrom
INSERT INTO trunking_messages (
    system_id, sys_num, sys_name, trunk_msg, trunk_msg_type,
    opcode, opcode_type, opcode_desc, meta, "time",
    instance_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
