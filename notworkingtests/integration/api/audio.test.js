const request = require('supertest');
const { TREngine } = require('../../../src/app');
const { getGridFSBucket } = require('../../../src/config/mongodb');
const mongoose = require('mongoose');
const fs = require('fs');
const path = require('path');

describe('Audio API Tests', () => {
  let app;
  let engine;
  let gridFSBucket;
  const testAudioPath = path.join(__dirname, '../../fixtures/test.wav');

  beforeAll(async () => {
    engine = new TREngine();
    await engine.initialize();
    app = engine.app;
    gridFSBucket = getGridFSBucket();

    // Create test audio file if it doesn't exist
    if (!fs.existsSync(testAudioPath)) {
      const testDir = path.dirname(testAudioPath);
      if (!fs.existsSync(testDir)) {
        fs.mkdirSync(testDir, { recursive: true });
      }
      // Create a simple WAV file
      const buffer = Buffer.alloc(44); // WAV header size
      fs.writeFileSync(testAudioPath, buffer);
    }
  });

  afterAll(async () => {
    await engine.shutdown();
    // Cleanup test file
    if (fs.existsSync(testAudioPath)) {
      fs.unlinkSync(testAudioPath);
    }
  });

  beforeEach(async () => {
    // Clear any existing files
    const files = await gridFSBucket.find({}).toArray();
    for (const file of files) {
      await gridFSBucket.delete(file._id);
    }
  });

  describe('Audio Retrieval', () => {
    it('gets audio file for specific call', async () => {
      // Upload test file
      const filename = 'test_call_123.wav';
      const uploadStream = gridFSBucket.openUploadStream(filename, {
        metadata: {
          sys_name: 'Test System',
          talkgroup: 101,
          unit: 1234
        }
      });
      const fileStream = fs.createReadStream(testAudioPath);
      await new Promise((resolve, reject) => {
        fileStream.pipe(uploadStream)
          .on('error', reject)
          .on('finish', resolve);
      });

      const res = await request(app)
        .get('/api/v1/audio/call/test_call_123.wav')
        .expect('Content-Type', /audio\/wav/)
        .expect(200);

      expect(res.headers['content-disposition']).toContain('test_call_123.wav');
    });

    it('gets audio metadata', async () => {
      // Upload test file
      const filename = 'test_call_123.wav';
      const uploadStream = gridFSBucket.openUploadStream(filename, {
        metadata: {
          sys_name: 'Test System',
          talkgroup: 101,
          unit: 1234,
          duration: 30
        }
      });
      const fileStream = fs.createReadStream(testAudioPath);
      await new Promise((resolve, reject) => {
        fileStream.pipe(uploadStream)
          .on('error', reject)
          .on('finish', resolve);
      });

      const res = await request(app)
        .get('/api/v1/audio/call/test_call_123.wav/metadata')
        .expect('Content-Type', /json/)
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.metadata.call_id).toBe('test_call_123');
      expect(res.body.metadata.formats.wav).toBeDefined();
    });
  });

  describe('Audio Archive', () => {
    it('searches archived recordings', async () => {
      // Upload multiple test files
      const files = [
        {
          filename: 'test_call_1.wav',
          metadata: {
            sys_name: 'System 1',
            talkgroup: 101,
            unit: 1234,
            duration: 30
          }
        },
        {
          filename: 'test_call_2.wav',
          metadata: {
            sys_name: 'System 2',
            talkgroup: 102,
            unit: 5678,
            duration: 45
          }
        }
      ];

      for (const file of files) {
        const uploadStream = gridFSBucket.openUploadStream(file.filename, {
          metadata: file.metadata
        });
        const fileStream = fs.createReadStream(testAudioPath);
        await new Promise((resolve, reject) => {
          fileStream.pipe(uploadStream)
            .on('error', reject)
            .on('finish', resolve);
        });
      }

      // Test search with filters
      const res = await request(app)
        .get('/api/v1/audio/archive')
        .query({
          sys_name: 'System 1',
          limit: 10
        })
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.files).toHaveLength(1);
      expect(res.body.data.pagination).toBeDefined();
      expect(res.body.data.files[0].metadata.sys_name).toBe('System 1');
    });

    it('supports pagination in archive search', async () => {
      // Upload multiple test files
      const files = Array.from({ length: 5 }, (_, i) => ({
        filename: `test_call_${i + 1}.wav`,
        metadata: {
          sys_name: 'Test System',
          talkgroup: 101,
          unit: 1234,
          duration: 30
        }
      }));

      for (const file of files) {
        const uploadStream = gridFSBucket.openUploadStream(file.filename, {
          metadata: file.metadata
        });
        const fileStream = fs.createReadStream(testAudioPath);
        await new Promise((resolve, reject) => {
          fileStream.pipe(uploadStream)
            .on('error', reject)
            .on('finish', resolve);
        });
      }

      // Test pagination
      const res = await request(app)
        .get('/api/v1/audio/archive')
        .query({
          limit: 2,
          offset: 2
        })
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.files).toHaveLength(2);
      expect(res.body.data.pagination.total).toBe(5);
      expect(res.body.data.pagination.has_more).toBe(true);
    });
  });
});
