<?php

namespace Plugin\PxboardMigrationBridge;

use RuntimeException;

class Store
{
    private static function path(int $machineId): string
    {
        if ($machineId < 1) {
            throw new RuntimeException('Invalid machine ID');
        }
        return storage_path('app/private/pxboard-migration-bridge/' . $machineId . '.json');
    }

    public static function read(int $machineId): ?array
    {
        $path = self::path($machineId);
        if (!is_file($path)) {
            return null;
        }
        $handle = fopen($path . '.lock', 'c+');
        if (!$handle || !flock($handle, LOCK_SH)) {
            throw new RuntimeException('Cannot read bridge task');
        }
        chmod($path . '.lock', 0600);
        try {
            $raw = file_get_contents($path);
            if ($raw === false) {
                throw new RuntimeException('Cannot read bridge task');
            }
            return $raw === '' ? null : json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        } finally {
            flock($handle, LOCK_UN);
            fclose($handle);
        }
    }

    public static function update(int $machineId, callable $change): array
    {
        $path = self::path($machineId);
        if (!is_dir(dirname($path)) && !mkdir(dirname($path), 0700, true) && !is_dir(dirname($path))) {
            throw new RuntimeException('Cannot create bridge task directory');
        }
        $handle = fopen($path . '.lock', 'c+');
        if (!$handle || !flock($handle, LOCK_EX)) {
            throw new RuntimeException('Cannot write bridge task');
        }
        chmod($path . '.lock', 0600);
        $temporary = null;
        try {
            $raw = is_file($path) ? file_get_contents($path) : '';
            if ($raw === false) {
                throw new RuntimeException('Cannot read bridge task');
            }
            $old = $raw === '' ? null : json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
            $record = $change($old);
            $encoded = json_encode($record, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
            $temporary = tempnam(dirname($path), '.task-');
            if ($temporary === false || !chmod($temporary, 0600)
                || file_put_contents($temporary, $encoded) !== strlen($encoded) || !rename($temporary, $path)) {
                throw new RuntimeException('Cannot persist bridge task');
            }
            $temporary = null;
            return $record;
        } finally {
            if ($temporary !== null && is_file($temporary)) {
                unlink($temporary);
            }
            flock($handle, LOCK_UN);
            fclose($handle);
        }
    }
}
