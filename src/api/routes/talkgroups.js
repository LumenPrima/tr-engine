const router = require('express').Router();
const { query, param, validationResult } = require('express-validator');
const TalkgroupManager = require('../../services/state/TalkgroupManager');
const errorHandler = require('../middleware/errorHandler');

const talkgroupManager = new TalkgroupManager();

// Get talkgroups with advanced filtering
router.get('/', 
  [
    query('sys_name').optional().isString(),
    query('sys_num').optional().isInt(),
    query('talkgroup').optional().isInt(),
    query('emergency').optional().isBoolean(),
    query('category').optional().isString(),
    query('group').optional().isString()
  ],
  async (req, res) => {
    try {
      const errors = validationResult(req);
      if (!errors.isEmpty()) {
        return res.status(400).json({ 
          status: 'error',
          errors: errors.array(),
          timestamp: new Date().toISOString()
        });
      }

      const talkgroups = await talkgroupManager.getTalkgroups(req.query);
      res.json({
        status: 'success',
        count: talkgroups.length,
        data: talkgroups,
        timestamp: new Date().toISOString()
      });
    } catch (err) {
      errorHandler(err, req, res);
    }
  }
);

// Get talkgroup by ID
router.get('/:id', 
  param('id').isMongoId(),
  async (req, res) => {
    try {
      const errors = validationResult(req);
      if (!errors.isEmpty()) {
        return res.status(400).json({
          status: 'error',
          errors: errors.array(),
          timestamp: new Date().toISOString()
        });
      }

      const talkgroup = await talkgroupManager.getTalkgroupById(req.params.id);
      
      if (!talkgroup) {
        return res.status(404).json({
          status: 'error',
          message: 'Talkgroup not found',
          timestamp: new Date().toISOString()
        });
      }
      
      res.json({
        status: 'success',
        data: talkgroup,
        timestamp: new Date().toISOString()
      });
    } catch (err) {
      errorHandler(err, req, res);
    }
  }
);

module.exports = router;
